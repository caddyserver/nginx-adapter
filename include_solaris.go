// +build solaris illumos

package nginxconf

func init() {
	// https://www.nginx.com/resources/wiki/start/topics/tutorials/solaris_11/
	nginxConfPrefix = "/opt/local/nginx"
	for k, v := range nginxConfDirs {
		if v == "conf.d/" {
			nginxConfDirs[k] = "conf/"
		}
	}
}
