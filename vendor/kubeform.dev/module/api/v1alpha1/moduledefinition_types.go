/*
Copyright AppsCode Inc. and Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiv1 "kmodules.xyz/client-go/api/v1"
)

type Git struct {
	Ref      string                 `json:"ref"`
	CheckOut *string                `json:"checkOut,omitempty"`
	Cred     *apiv1.ObjectReference `json:"cred,omitempty"`
}

type ModuleRef struct {
	Git Git `json:"git,omitempty"`
}

type Provider struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// ModuleDefinitionSpec defines the desired state of ModuleDefinition
type ModuleDefinitionSpec struct {
	Schema    v1.JSONSchemaProps `json:"schema"`
	ModuleRef ModuleRef          `json:"moduleRef"`
	Provider  Provider           `json:"provider"`
}

// ModuleDefinitionStatus defines the observed state of ModuleDefinition
type ModuleDefinitionStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// ModuleDefinition is the Schema for the moduledefinitions API
type ModuleDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModuleDefinitionSpec   `json:"spec,omitempty"`
	Status ModuleDefinitionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ModuleDefinitionList contains a list of ModuleDefinition
type ModuleDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModuleDefinition `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ModuleDefinition{}, &ModuleDefinitionList{})
}
