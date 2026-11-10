package configures

type MindlinkConfig struct {
	Enabled                bool                          `yaml:"enabled"`
	Trusts                 []MindlinkTrustConfig         `yaml:"trusts"`
	DisableAuthoritySource MindlinkAuthoritySourceConfig `yaml:"disable_authority_source"`
}

type MindlinkAuthoritySourceConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Program    string `yaml:"program"`
	ConfigPath string `yaml:"config_path"`
}

type MindlinkTrustConfig struct {
	Issuer             string   `yaml:"issuer"`
	KeyID              string   `yaml:"kid"`
	Audience           string   `yaml:"audience"`
	Environment        string   `yaml:"environment"`
	TenantID           string   `yaml:"tenant_id"`
	AppKey             string   `yaml:"appkey"`
	SystemActorID      string   `yaml:"system_actor_id"`
	PublicKey          string   `yaml:"public_key_ed25519_base64"`
	QueryOperations    []string `yaml:"query_operations"`
	MutationOperations []string `yaml:"mutation_operations"`
}
