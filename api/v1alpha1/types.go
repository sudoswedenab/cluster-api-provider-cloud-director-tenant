package v1alpha1

type AdditionalDisk struct {
	Name    string `json:"name,omitempty"`
	SizeMiB int64  `json:"sizeMiB"`
}
