package activation

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = 1

type Name string

const (
	RegistrationStarted   Name = "registration_started"
	RegistrationCompleted Name = "registration_completed"
	WorkspaceCreated      Name = "workspace_created"
	OnboardingStepViewed  Name = "onboarding_step_viewed"
	OnboardingStepClicked Name = "onboarding_step_clicked"
	MenuItemCreated       Name = "menu_item_created"
	TableCreated          Name = "table_created"
	QRPreviewed           Name = "qr_previewed"
	PaymentConfigured     Name = "payment_configured"
	StaffInvited          Name = "staff_invited"
	SetupCompleted        Name = "setup_completed"
	TestOrderCompleted    Name = "test_order_completed"
	ActivationAchieved    Name = "activation_achieved"
	FirstPaidBill         Name = "first_paid_bill"
)

type Denominator string

const (
	ConsentQualifiedClient Denominator = "consent_qualified_client"
	ServerAllEligible      Denominator = "server_all_eligible"
)

type Definition struct {
	Name                Name
	ServerAuthoritative bool
	Denominator         Denominator
}

var catalog = []Definition{
	{Name: RegistrationStarted, Denominator: ConsentQualifiedClient},
	{Name: RegistrationCompleted, ServerAuthoritative: true, Denominator: ServerAllEligible},
	{Name: WorkspaceCreated, ServerAuthoritative: true, Denominator: ServerAllEligible},
	{Name: OnboardingStepViewed, Denominator: ConsentQualifiedClient},
	{Name: OnboardingStepClicked, Denominator: ConsentQualifiedClient},
	{Name: MenuItemCreated, ServerAuthoritative: true, Denominator: ServerAllEligible},
	{Name: TableCreated, ServerAuthoritative: true, Denominator: ServerAllEligible},
	{Name: QRPreviewed, ServerAuthoritative: true, Denominator: ServerAllEligible},
	{Name: PaymentConfigured, ServerAuthoritative: true, Denominator: ServerAllEligible},
	{Name: StaffInvited, ServerAuthoritative: true, Denominator: ServerAllEligible},
	{Name: SetupCompleted, ServerAuthoritative: true, Denominator: ServerAllEligible},
	{Name: TestOrderCompleted, ServerAuthoritative: true, Denominator: ServerAllEligible},
	{Name: ActivationAchieved, ServerAuthoritative: true, Denominator: ServerAllEligible},
	{Name: FirstPaidBill, ServerAuthoritative: true, Denominator: ServerAllEligible},
}

var definitions = func() map[Name]Definition {
	result := make(map[Name]Definition, len(catalog))
	for _, definition := range catalog {
		result[definition.Name] = definition
	}
	return result
}()

func Names() []Name {
	result := make([]Name, 0, len(catalog))
	for _, definition := range catalog {
		result = append(result, definition.Name)
	}
	return result
}

func DefinitionFor(name Name) (Definition, bool) {
	definition, ok := definitions[name]
	return definition, ok
}

func MustDefinition(name Name) Definition {
	definition, ok := DefinitionFor(name)
	if !ok {
		panic("unknown activation event: " + string(name))
	}
	return definition
}

type Origin string

const (
	ClientOrigin Origin = "client"
	ServerOrigin Origin = "server"
)

type Dimensions struct {
	BusinessID          string `json:"business_id,omitempty"`
	Locale              string `json:"locale"`
	DeviceClass         string `json:"device_class"`
	AcquisitionSource   string `json:"acquisition_source,omitempty"`
	AcquisitionCampaign string `json:"acquisition_campaign,omitempty"`
	OnboardingStep      string `json:"onboarding_step,omitempty"`
	ElapsedMS           int64  `json:"elapsed_ms"`
}

type Event struct {
	Name           Name           `json:"name"`
	SchemaVersion  int            `json:"schema_version"`
	BusinessID     uint           `json:"business_id,omitempty"`
	FunnelID       string         `json:"funnel_id,omitempty"`
	IdempotencyKey string         `json:"idempotency_key"`
	Dimensions     Dimensions     `json:"dimensions"`
	OccurredAt     time.Time      `json:"occurred_at"`
	Extra          map[string]any `json:"-"`
}

var (
	safeTokenPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,79}$`)
	uuidPattern           = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	cardPattern           = regexp.MustCompile(`(?:\d[ -]*?){13,19}`)
	idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,254}$`)
	forbiddenKeys         = []string{"name", "email", "address", "customer", "menu", "payment", "card", "amount", "price", "token", "tx_hash", "wallet", "phone"}
)

var ErrInvalidEvent = errors.New("invalid activation event")

func Validate(event Event, origin Origin) error {
	definition, ok := DefinitionFor(event.Name)
	if !ok {
		return fmt.Errorf("%w: unknown event", ErrInvalidEvent)
	}
	if event.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: unsupported schema version", ErrInvalidEvent)
	}
	if !idempotencyKeyPattern.MatchString(event.IdempotencyKey) || strings.Contains(event.IdempotencyKey, "@") || cardPattern.MatchString(event.IdempotencyKey) {
		return fmt.Errorf("%w: invalid idempotency key", ErrInvalidEvent)
	}
	if definition.ServerAuthoritative && origin != ServerOrigin {
		return fmt.Errorf("%w: authoritative event requires server confirmation", ErrInvalidEvent)
	}
	if origin == ClientOrigin && !uuidPattern.MatchString(strings.ToLower(event.FunnelID)) {
		return fmt.Errorf("%w: invalid funnel id", ErrInvalidEvent)
	}
	if origin == ClientOrigin {
		if event.Dimensions.BusinessID != "" {
			return fmt.Errorf("%w: client events cannot assert business_id", ErrInvalidEvent)
		}
		step := event.Dimensions.OnboardingStep
		if step == "" {
			step = "global"
		}
		expected := fmt.Sprintf("client:%s:%s:%s", event.FunnelID, event.Name, step)
		if event.IdempotencyKey != expected {
			return fmt.Errorf("%w: invalid client idempotency key", ErrInvalidEvent)
		}
	}
	if origin == ServerOrigin && event.BusinessID == 0 {
		return fmt.Errorf("%w: business id required", ErrInvalidEvent)
	}
	if event.Dimensions.ElapsedMS < 0 {
		return fmt.Errorf("%w: elapsed_ms must be nonnegative", ErrInvalidEvent)
	}
	if err := validateSafeDimensions(event.Dimensions); err != nil {
		return err
	}
	if len(event.Extra) > 0 {
		keys := make([]string, 0, len(event.Extra))
		for key := range event.Extra {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return fmt.Errorf("%w: unsupported properties: %s", ErrInvalidEvent, strings.Join(keys, ","))
	}
	return nil
}

func validateSafeDimensions(dimensions Dimensions) error {
	values := map[string]string{
		"business_id":          dimensions.BusinessID,
		"locale":               dimensions.Locale,
		"device_class":         dimensions.DeviceClass,
		"acquisition_source":   dimensions.AcquisitionSource,
		"acquisition_campaign": dimensions.AcquisitionCampaign,
		"onboarding_step":      dimensions.OnboardingStep,
	}
	for key, value := range values {
		if value == "" && (key == "business_id" || key == "acquisition_source" || key == "acquisition_campaign" || key == "onboarding_step") {
			continue
		}
		lower := strings.ToLower(value)
		if strings.Contains(value, "@") || cardPattern.MatchString(value) || !safeTokenPattern.MatchString(value) {
			return fmt.Errorf("%w: unsafe %s", ErrInvalidEvent, key)
		}
		for _, forbidden := range forbiddenKeys {
			if strings.Contains(lower, forbidden+":") || strings.Contains(lower, forbidden+"=") {
				return fmt.Errorf("%w: unsafe %s", ErrInvalidEvent, key)
			}
		}
	}
	return nil
}
