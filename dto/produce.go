package dto

type PublishRequest struct {
	Subject string `json:"subject"`
	Message string `json:"message"`
}
