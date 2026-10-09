package auth

// verifyEmailToken drives the live verification entry point and drops the
// returned identity for tests that only assert the error.
func verifyEmailToken(s *AuthService, token string) error {
	_, err := s.VerifyEmailIdentity(token)
	return err
}
