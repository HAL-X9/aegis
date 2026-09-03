package schema

type RateLimit struct {
	Rate  float64 `yaml:"rate"`
	Burst uint32  `yaml:"burst"`
}
