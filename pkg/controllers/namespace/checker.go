package namespace

type NamespaceChecker func(string) bool

func SingleNamespaceChecker(singleNamespace string) NamespaceChecker {
	return func(namespace string) bool {
		return namespace == singleNamespace
	}
}
