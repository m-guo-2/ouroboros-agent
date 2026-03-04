package modules

func buildMomentActions() map[string]string {
	return map[string]string{
		"list":           "moment/list",
		"batch-detail":   "moment/getDetailBatch",
		"upload-media":   "moment/upload",
		"publish":        "moment/publish",
		"delete":         "moment/delete",
		"like":           "moment/like",
		"comment":        "moment/comment",
		"delete-comment": "moment/deleteComment",
	}
}
