package v0alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type SecureValue struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec SecureValueSpec `json:"spec,omitempty"`
}

type SecureValueSpec struct {
	// Visible title for this secret
	Title string `json:"title"`

	// The raw value is only valid for write.  Read/List will always be empty
	// Writing with an empty value will always fail
	Value string `json:"value,omitempty"`

	// The APIs that are allowed to decrypt this secret
	APIs []string `json:"apis"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type SecureValueList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []SecureValue `json:"items,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type SecureValueActivityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []SecureValueActivity `json:"items,omitempty"`
}

type SecureValueActivity struct {
	Timestamp metav1.Timestamp `json:"timestamp"`
	Action    string           `json:"action"` // CREATE, UPDATE, DELETE, etc
	Identity  string           `json:"identity"`
	Details   string           `json:"details,omitempty"`
}
