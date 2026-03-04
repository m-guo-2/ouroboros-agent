package modules

func buildContactActions() map[string]string {
	return map[string]string{
		"batch-detail":          "contact/getDetailBatch",
		"list-external":         "contact/listExternal",
		"list-internal":         "contact/listInternal",
		"search":                "contact/search",
		"add-personal-wechat":   "contact/addPersonalWechat",
		"add-enterprise-wechat": "contact/addEnterpriseWechat",
		"add-wechat-card":       "contact/addWechatCard",
		"re-add":                "contact/reAdd",
		"approve-request":       "contact/approveRequest",
		"update-personal":       "contact/updatePersonal",
		"update-enterprise":     "contact/updateEnterprise",
		"delete":                "contact/delete",
		"get-openid":            "contact/getOpenId",
	}
}
