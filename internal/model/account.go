package model

// Account is X-port's core unit: one account owns one Xray inbound and one
// dedicated public listening port.
type Account struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Enabled            bool   `json:"enabled"`
	Listen             string `json:"listen"`
	Port               int    `json:"port"`
	Protocol           string `json:"protocol"`
	SettingsJSON       string `json:"-"`
	StreamSettingsJSON string `json:"-"`
	SniffingJSON       string `json:"-"`
	Tag                string `json:"tag"`
	UpBytes            int64  `json:"upBytes"`
	DownBytes          int64  `json:"downBytes"`
	QuotaBytes         int64  `json:"quotaBytes"`
	AllTimeBytes       int64  `json:"allTimeBytes"`
	ExpiryTime         int64  `json:"expiryTime"`
	CreatedAt          int64  `json:"createdAt"`
	UpdatedAt          int64  `json:"updatedAt"`
}

type Admin struct {
	Username     string
	PasswordHash string
}
