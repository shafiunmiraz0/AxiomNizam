package models

// Notification resource domain types.

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const (
	ChannelKind       = "NotificationChannel"
	ChannelAPIVersion = "notification.axiomnizam.io/v1"
)

type ChannelSpec struct {
	ChannelType string `json:"channelType"` // discord, slack, email, webhook
	WebhookURL  string `json:"webhookUrl,omitempty"`
	Enabled     bool   `json:"enabled"`
}

type ChannelResourceStatus struct {
	resources.ObjectStatus `json:",inline"`
	ChannelActive          bool `json:"channelActive"`
}

type ChannelResource struct {
	resources.TypeMeta   `json:",inline"`
	resources.ObjectMeta `json:"metadata"`
	Spec                 ChannelSpec           `json:"spec"`
	Status               ChannelResourceStatus `json:"status"`
}

func (r *ChannelResource) GetObjectMeta() *resources.ObjectMeta { return &r.ObjectMeta }
func (r *ChannelResource) GetTypeMeta() *resources.TypeMeta     { return &r.TypeMeta }
func (r *ChannelResource) GetStatus() *resources.ObjectStatus   { return &r.Status.ObjectStatus }
func (r *ChannelResource) SetStatus(s *resources.ObjectStatus) {
	if s != nil {
		r.Status.ObjectStatus = *s
	}
}
func (r *ChannelResource) DeepCopy() resources.Resource { cp := *r; return &cp }
func (r *ChannelResource) GetKey() string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func (r *ChannelResource) GetGeneration() int64         { return r.Generation }
func (r *ChannelResource) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }
