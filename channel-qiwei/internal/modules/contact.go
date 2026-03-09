package modules

func buildContactActions() map[string]string {
	return map[string]string{
		"batch-detail":          "/contact/batchGetUserinfo",
		"list-external":         "/contact/getWxContactList",
		"list-internal":         "/contact/getWxWorkContactList",
		"search":                "/contact/searchContact",
		"add-personal-wechat":   "/contact/addSearchWxContact",
		"add-enterprise-wechat": "/contact/addSearchWxWorkContact",
		"add-wechat-card":       "/contact/addCardContact",
		"re-add":                "/contact/addDeletedContact",
		"approve-request":       "/contact/agreeContact",
		"update-personal":       "/contact/updateWxContact",
		"update-enterprise":     "/contact/updateWxWorkContact",
		"delete":                "/contact/deleteContact",
		"get-openid":            "/contact/openid",
	}
}
