package plugins

import "testing"

type optionalInterfaceFixture struct{}

func (optionalInterfaceFixture) NormalizeConfig(uint, map[string]interface{}) (map[string]interface{}, error) {
	return nil, nil
}
func (optionalInterfaceFixture) PublicConfig(map[string]interface{}) map[string]interface{} {
	return nil
}
func (optionalInterfaceFixture) AttachAlternativePayment(uint, string, uint) error {
	return nil
}
func (optionalInterfaceFixture) GetPaymentStatusDetails(uint, string) (*PaymentStatusDetails, error) {
	return nil, nil
}

func TestOptionalPluginInterfacesCompile(t *testing.T) {
	var _ ConfigNormalizer = optionalInterfaceFixture{}
	var _ PublicConfigProvider = optionalInterfaceFixture{}
	var _ AlternativePaymentTracker = optionalInterfaceFixture{}
	var _ DetailedPaymentStatusProvider = optionalInterfaceFixture{}
}
