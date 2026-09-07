package domain

type OAuthProvider string

const (
	OAuthProviderGoogle OAuthProvider = "GOOGLE"
	OAuthProviderGitHub OAuthProvider = "GITHUB"
)

type OAuthLoginStart struct {
	AuthURL  string
	State    string
	Verifier string
	Provider OAuthProvider
}

// DeviceLoginStart is the result of initiating an OAuth device-authorization flow.
type DeviceLoginStart struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	ExpiresIn       int
	Interval        int
}

// DeviceAuthStatus reports where a polled device-flow login stands.
type DeviceAuthStatus string

const (
	DeviceAuthPending  DeviceAuthStatus = "pending"
	DeviceAuthSlowDown DeviceAuthStatus = "slow_down"
	DeviceAuthComplete DeviceAuthStatus = "complete"
)

type OAuthProfile struct {
	Provider      OAuthProvider
	ProviderID    string
	Email         string
	DisplayName   string
	AvatarURL     string
	EmailVerified bool
}
