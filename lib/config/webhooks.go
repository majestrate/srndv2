package config

// configuration for a single web hook
type WebhookConfig struct {
	// user provided name for this hook
	Name string `json:"name"`
	// callback URL for webhook
	URL string `json:"url"`
}

var DefaultWebHookConfig = &WebhookConfig{
	Name: "vichan",
	URL:  "http://localhost/webhook.php",
}
