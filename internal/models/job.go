package models

type Job struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	RestartPolicy string `json:"restart_policy"`
	Done      int    `json:"done"`
	Total     int    `json:"total"`
	Payload   string `json:"payload"`
	Result    string `json:"result"`
	Error     string `json:"error"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
