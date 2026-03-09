package modules

func buildMomentActions() map[string]string {
	return map[string]string{
		"list":           "/sns/getSnsRecord",
		"batch-detail":   "/sns/getSnsDetail",
		"upload-media":   "/sns/upload",
		"publish":        "/sns/postSns",
		"delete":         "/sns/deleteSns",
		"like":           "/sns/snsLike",
		"comment":        "/sns/snsComment",
		"delete-comment": "/sns/deleteSnsComment",
	}
}
