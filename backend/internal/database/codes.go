package database

// No imports needed for this file

// SaveCode records a new code in the database
func SaveCode(code *Code) error {
	result := db.Create(code)
	return result.Error
}

// GetCode retrieves a code by its code
func GetCode(code string) (*Code, error) {
	var codeRecord Code
	result := db.Where("code = ?", code).First(&codeRecord)
	if result.Error != nil {
		return nil, result.Error
	}

	return &codeRecord, nil
}

// UpdateCode updates the status of a code
func UpdateCode(code *Code) error {
	result := db.Where("code = ?", code.Code).Updates(code)
	return result.Error
}

// DeleteCode deletes a code from the database
func DeleteCode(code string) error {
	result := db.Where("code = ?", code).Delete(&Code{})
	return result.Error
}

// GetAllCodes retrieves all codes from the database
func GetAllCodes() ([]Code, error) {
	var codes []Code
	result := db.Find(&codes)
	if result.Error != nil {
		return nil, result.Error
	}

	return codes, nil
}
