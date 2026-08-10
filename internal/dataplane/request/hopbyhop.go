package request

import "net/http"

func RemoveHopHeaders(headers http.Header) {
	for name, value := range headers {
		switch name {
		case "Connection":
			for _, header := range value {
				headers.Del(header)
			}
			headers.Del(name)
		case
			"Keep-Alive",
			"Proxy-Authenticate",
			"Proxy-Authorization",
			"Proxy-Connection",
			"TE",
			"Trailer",
			"Transfer-Encoding",
			"Upgrade":

			headers.Del(name)
		}
	}
}
