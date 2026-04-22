package model

type Profile string

const (
	ProfileObserve Profile = "observe"
	ProfileOperate Profile = "operate"
	ProfileReview  Profile = "review"
	ProfileSidecar Profile = "sidecar"
	ProfileAdmin   Profile = "admin"
)

type ToolAttributes struct {
	RequiresModel   bool      `json:"requires_model"`
	RequiresAdapter bool      `json:"requires_adapter"`
	AllowedProfiles []Profile `json:"allowed_profiles,omitempty"`
}

func (a ToolAttributes) Allows(profile Profile) bool {
	if len(a.AllowedProfiles) == 0 {
		return true
	}
	for _, allowed := range a.AllowedProfiles {
		if allowed == profile {
			return true
		}
	}
	return false
}
