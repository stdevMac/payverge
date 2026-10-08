package emails

import (
	"os"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/config"
)

// defaultEmailLogoPath is the wordmark the frontend serves from public/;
// emails load it from the instance's own PUBLIC_URL unless LOGO_URL is set.
const defaultEmailLogoPath = "/images/PayvergeLogo.png"

// addInstanceTemplateFields injects the instance identity every layout and
// content template may reference. Values come from the instance env
// (PRODUCT_NAME, COMPANY_NAME, COMPANY_ADDRESS, PUBLIC_URL, LOGO_URL);
// nothing points at the upstream deployment.
//
//	product_name       PRODUCT_NAME (default Payverge)
//	company_name       COMPANY_NAME, else product_name
//	company_address    COMPANY_ADDRESS (postal address for CAN-SPAM footers;
//	                   omitted from the footer when empty)
//	instance_url       PUBLIC_URL origin
//	instance_host      PUBLIC_URL host, shown as the footer link text
//	instance_logo_url  absolute logo URL for the layout header
func addInstanceTemplateFields(templateBody map[string]interface{}) {
	templateBody["product_name"] = config.ProductName()
	templateBody["company_name"] = config.CompanyName()
	templateBody["company_address"] = strings.TrimSpace(os.Getenv("COMPANY_ADDRESS"))
	templateBody["instance_url"] = config.PublicURL()
	templateBody["instance_host"] = config.PublicHost()
	templateBody["instance_logo_url"] = emailLogoURL()
}

// emailLogoURL resolves the header logo to an absolute URL (mail clients
// cannot resolve relative paths): an absolute LOGO_URL as-is, a root-relative
// LOGO_URL under PUBLIC_URL, else the bundled wordmark under PUBLIC_URL.
func emailLogoURL() string {
	logo := config.LogoURL()
	lower := strings.ToLower(logo)
	switch {
	case strings.HasPrefix(lower, "https://"), strings.HasPrefix(lower, "http://"):
		return logo
	case strings.HasPrefix(logo, "/"):
		return config.PublicURL() + logo
	default:
		return config.PublicURL() + defaultEmailLogoPath
	}
}
