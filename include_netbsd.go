// +build netbsd

package nginxconf

func init() {
	// https://www.netbsd.mx/nginx-php.html
	nginxConfPrefix = "/usr/pkg/etc/nginx"
}
