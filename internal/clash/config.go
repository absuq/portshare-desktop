package clash

import "gopkg.in/yaml.v3"

type rawConfig struct {
	MixedPort              int    `yaml:"mixed-port"`
	SocksPort              int    `yaml:"socks-port"`
	HTTPPort               int    `yaml:"port"`
	ExternalController     string `yaml:"external-controller"`
	ExternalControllerPipe string `yaml:"external-controller-pipe"`
	Secret                 string `yaml:"secret"`
	AllowLAN               bool   `yaml:"allow-lan"`
	TUN                    struct {
		Enable bool `yaml:"enable"`
	} `yaml:"tun"`
}

func ParseConfigYAML(raw []byte) (ClashConfig, error) {
	var cfg rawConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return ClashConfig{}, err
	}
	return ClashConfig{
		MixedPort:              cfg.MixedPort,
		SocksPort:              cfg.SocksPort,
		HTTPPort:               cfg.HTTPPort,
		ExternalController:     cfg.ExternalController,
		ExternalControllerPipe: cfg.ExternalControllerPipe,
		Secret:                 cfg.Secret,
		AllowLAN:               cfg.AllowLAN,
		TUNEnabled:             cfg.TUN.Enable,
	}, nil
}

func MaskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	return "***"
}
