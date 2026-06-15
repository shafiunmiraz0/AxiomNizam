package conductor

import (
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/conductor/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/dualwrite"
	"axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"axiomnizam.bitbd.net/axiomnizam/internal/resources"
)

const conductorDWModule = "conductor"

type ProducerDualWriteStore = store.ResourceStore[*models.ProducerResource]

func (h *Handler) SetProducerDualWriteStore(s ProducerDualWriteStore) { h.producerDualWriteStore = s }

func (h *Handler) dualWriteProducer(p *Producer) {
	if h.producerDualWriteStore == nil || p == nil {
		return
	}
	resource := &models.ProducerResource{
		TypeMeta:   resources.TypeMeta{APIVersion: models.ProducerAPIVersion, Kind: models.ProducerKind},
		ObjectMeta: resources.ObjectMeta{Name: p.ID, Namespace: "default", Generation: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		Spec: models.ProducerSpec{
			Backend:     p.Backend,
			Exchange:    p.Exchange,
			RoutingKey:  p.RoutingKey,
			Topic:       p.Topic,
			ContentType: p.ContentType,
			Headers:     p.Headers,
			Config:      p.Config,
			Active:      p.Status == StatusActive,
		},
		Status: models.ProducerResourceStatus{ObjectStatus: resources.ObjectStatus{Phase: p.Status}, ProducerStatus: p.Status, MessagesSent: p.MessagesSent},
	}
	dualwrite.Write(conductorDWModule, h.producerDualWriteStore, resource)
}
