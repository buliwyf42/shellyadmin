package models

type CredentialGroup struct {
	Name     string   `json:"name"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	HA1      string   `json:"ha1"`
	Tags     []string `json:"tags"`
}

type DeviceCredentialGroupAssignment struct {
	MAC       string `json:"mac"`
	GroupName string `json:"group_name"`
}
