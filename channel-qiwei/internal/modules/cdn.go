package modules

func buildCDNActions() map[string]string {
	return map[string]string{
		"upload-async":            "cdn/uploadAsync",
		"upload":                  "cdn/upload",
		"upload-url":              "cdn/uploadUrl",
		"download-qw-file":        "cdn/downloadQwFile",
		"download-qw-file-async":  "cdn/downloadQwFileAsync",
		"download-qw-large-async": "cdn/downloadQwLargeFileAsync",
		"download-gw-file":        "cdn/downloadGwFile",
		"cdn-to-url":              "cdn/cdnToUrl",
		"download-gw-async":       "cdn/downloadGwFileAsync",
	}
}
