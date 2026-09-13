package service

type CreatePayload struct {
	Email    string
	FullName string

	// PasswordHash is already hashed when it arrives. The auth module owns the
	// credential policy and does the hashing; this module stores the result and
	// never receives a plaintext password.
	//
	// Empty for an account created through the admin CRUD path, which sets no
	// password at all — such an account cannot sign in until one is set.
	PasswordHash string
}

type UpdatePayload struct {
	FullName string
}
