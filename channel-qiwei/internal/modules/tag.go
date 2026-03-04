package modules

func buildTagActions() map[string]string {
	return map[string]string{
		"list":              "tag/list",
		"edit-personal-tag": "tag/editPersonal",
		"edit-customer-tag": "tag/editCustomer",
	}
}
