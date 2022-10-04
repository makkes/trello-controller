package v1alpha1

import (
	"github.com/fluxcd/pkg/apis/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	FinalizerName = "trello.e13.dev/finalizer"
	CredentialsApiKey = "api-key"
	CredentialsApiToken = "api-token"
)

// TrelloConfigSpec defines the desired state of TrelloConfig
type TrelloConfigSpec struct {
	// +required
	Target metav1.TypeMeta `json:"target"`
	// +required
	ListID string `json:"listID"`
	// +required
	SecretRef meta.LocalObjectReference `json:"secretRef"`
}

type WatchSpec struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// TrelloConfigStatus defines the observed state of TrelloConfig
type TrelloConfigStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// TrelloConfig is the Schema for the trelloconfigs API
type TrelloConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TrelloConfigSpec   `json:"spec,omitempty"`
	Status TrelloConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TrelloConfigList contains a list of TrelloConfig
type TrelloConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TrelloConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TrelloConfig{}, &TrelloConfigList{})
}
