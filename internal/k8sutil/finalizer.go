package k8sutil

const openZroFinalizer = "finalizers.openzro.io"

func Finalizer(kind string) string {
	return openZroFinalizer + "/" + kind
}
