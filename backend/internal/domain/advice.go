package domain

type Advice struct {
	Stage     string   `json:"stage"`
	RiskLevel string   `json:"riskLevel"`
	RiskScore int      `json:"riskScore"`
	Metrics   []Metric `json:"metrics"`
	Actions   []Action `json:"actions"`
	Reasoning []string `json:"reasoning"`
}

type Metric struct {
	Key   string      `json:"key"`
	Label string      `json:"label"`
	Value interface{} `json:"value"`
	Unit  string      `json:"unit"`
}

type Action struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	Title      string      `json:"title"`
	Detail     string      `json:"detail"`
	Priority   int         `json:"priority"`
	Navigation *Navigation `json:"navigation,omitempty"`
}

type Navigation struct {
	Provider    string `json:"provider"`
	URI         string `json:"uri"`
	FallbackURL string `json:"fallbackUrl,omitempty"`
}

type JourneySnapshot struct {
	Journey     Journey    `json:"journey"`
	State       WorldState `json:"state"`
	Advice      Advice     `json:"advice"`
	GeneratedAt string     `json:"generatedAt"`
}
