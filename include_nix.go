// +build !windows !freebsd !netbsd !solaris !illumos

package nginxconf

func init() {
	nginxConfPrefix = "/etc/nginx"
}
