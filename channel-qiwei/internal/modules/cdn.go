package modules

func buildCDNActions() map[string]string {
	return map[string]string{
		"upload-async":            "/cloud/cdnUploadByUrlAsync",
		"upload":                  "/cloud/cdnBigUpload",
		"upload-url":              "/cloud/cdnBigUploadByUrl",
		"download-qw-file":        "/cloud/wxWorkDownload",
		"download-qw-file-async":  "/cloud/wxWorkDownloadAsync",
		"download-qw-large-async": "/cloud/cdnBigFileDownloadByUrlAsync",
		"download-gw-file":        "/cloud/wxDownload",
		"cdn-to-url":              "/cloud/cdnWxDownload",
		"download-gw-async":       "/cloud/wxDownloadAsync",
	}
}
