package main

type forwardWebhook struct {
	From              string `json:"from"`
	Type              string `json:"type"`
	ConnectionID      string `json:"connectionId,omitempty"`
	ProviderConfigKey string `json:"providerConfigKey"`
	Payload           any    `json:"payload"`
}

type authWebhook struct {
	From              string `json:"from"`
	Type              string `json:"type"`
	ConnectionID      string `json:"connectionId"`
	AuthMode          string `json:"authMode"`
	ProviderConfigKey string `json:"providerConfigKey"`
	Provider          string `json:"provider"`
	Environment       string `json:"environment"`
	Operation         string `json:"operation"`
	Success           bool   `json:"success"`
	ErrorMsg          string `json:"error,omitempty"`
}

type wsAck struct {
	MessageType string `json:"message_type"`
	WSClientID  string `json:"ws_client_id"`
}

type wsSuccess struct {
	MessageType       string `json:"message_type"`
	ProviderConfigKey string `json:"provider_config_key"`
	ConnectionID      string `json:"connection_id"`
	IsPending         bool   `json:"is_pending"`
}

type wsErr struct {
	MessageType       string `json:"message_type"`
	ProviderConfigKey string `json:"provider_config_key"`
	ConnectionID      string `json:"connection_id,omitempty"`
	ErrorType         string `json:"error_type"`
	ErrorDesc         string `json:"error_desc"`
}

type apiResp struct {
	Data any `json:"data,omitempty"`
}
