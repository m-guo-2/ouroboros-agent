package modules

func buildTagActions() map[string]string {
	return map[string]string{
		"list":              "/label/syncLabelList",
		"edit-personal-tag": "/label/editLabel",
		"edit-customer-tag": "/label/contactEditLabel",
	}
}
