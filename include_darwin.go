// +build darwin

package nginxconf

func init() {
	// https://www.cyberciti.biz/faq/freebsd-install-nginx-webserver/
	nginxConfPrefix = "/usr/local/etc/nginx"
}
