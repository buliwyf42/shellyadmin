package models

type Credential struct {
	Name     string   `json:"name"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	HA1      string   `json:"ha1"`
	Tags     []string `json:"tags"`
}
