package cmd

import "google.golang.org/protobuf/reflect/protoreflect"

// ForgetMapSplit undoes SplitMapOnLastEquals, for a test's cleanup.
func ForgetMapSplit(field protoreflect.FullName) { lastEqualsFields.Delete(field) }
