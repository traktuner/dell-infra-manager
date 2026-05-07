package models

type BiosAttributes struct {
	Attributes map[string]interface{} `json:"Attributes"`
}

type BiosRegistryEntry struct {
	AttributeName string        `json:"attribute_name"`
	DisplayName   string        `json:"display_name"`
	Type          string        `json:"type"`   // String, Integer, Enumeration, Boolean
	ReadOnly      bool          `json:"read_only"`
	CurrentValue  interface{}   `json:"current_value,omitempty"`
	AllowedValues []interface{} `json:"allowed_values,omitempty"`
	LowerBound    *int          `json:"lower_bound,omitempty"`
	UpperBound    *int          `json:"upper_bound,omitempty"`
}

type BiosSettingsRequest struct {
	Attributes map[string]interface{} `json:"attributes" binding:"required"`
	ApplyTime  string                 `json:"apply_time"` // OnReset (default)
}
