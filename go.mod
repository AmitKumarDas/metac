module openebs.io/metac

go 1.12

require (
	contrib.go.opencensus.io/exporter/prometheus v0.1.0
	github.com/ghodss/yaml v0.0.0-20150909031657-73d445a93680
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/google/go-cmp v0.3.0
	github.com/google/go-jsonnet v0.14.0
	github.com/pkg/errors v0.8.1
	go.opencensus.io v0.21.0
	gopkg.in/yaml.v2 v2.2.4
	k8s.io/api v0.0.0-20191005115622-2e41325d9e4b
	k8s.io/apiextensions-apiserver v0.0.0-20191008120836-c5dfed5b5134
	k8s.io/apimachinery v0.0.0-20191006235458-f9f2f3f8ab02
	k8s.io/client-go v0.0.0-20191008115822-1210218b4a26
	k8s.io/code-generator v0.0.0-20191003035328-700b1226c0bd
	k8s.io/klog v1.0.0
)

replace (
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20181025213731-e84da0312774
	golang.org/x/lint => golang.org/x/lint v0.0.0-20181217174547-8f45f776aaf1
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20190402181905-9f3314589c9a
	golang.org/x/sync => golang.org/x/sync v0.0.0-20181108010431-42b317875d0f
	golang.org/x/sys => golang.org/x/sys v0.0.0-20190209173611-3b5209105503
	golang.org/x/text => golang.org/x/text v0.3.1-0.20181227161524-e6919f6577db
	golang.org/x/time => golang.org/x/time v0.0.0-20161028155119-f51c12702a4d
	k8s.io/api => k8s.io/api v0.0.0-20191005115622-2e41325d9e4b
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20191005115455-e71eb83a557c
	k8s.io/client-go => k8s.io/client-go v0.0.0-20191005115821-b1fd78950135
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20191003035328-700b1226c0bd
)
