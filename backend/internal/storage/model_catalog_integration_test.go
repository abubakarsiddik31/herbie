package storage

import (
	"context"
	"testing"
)

func TestModelCatalogRoundTrip(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	catalog := NewModelCatalog(pool)

	rows, err := catalog.List(ctx)
	if err != nil {
		t.Fatalf("list seeded catalog: %v", err)
	}
	if len(rows) < 10 {
		t.Fatalf("seeded catalog = %d rows", len(rows))
	}

	const id = "test-admin-model"
	t.Cleanup(func() {
		_ = catalog.Delete(context.Background(), id)
		_ = catalog.Delete(context.Background(), id+"-b")
	})
	saved, err := catalog.Upsert(ctx, ModelCatalogRow{
		ID: id, Provider: "gemini", Label: "Test Admin Model",
		InputPerM: 0.5, OutputPerM: 1.5, Enabled: false,
		Capabilities: []byte(`{"reasoning":true}`),
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if saved.ID != id || saved.Enabled {
		t.Fatalf("saved = %+v", saved)
	}
	saved, err = catalog.Upsert(ctx, ModelCatalogRow{
		ID: id, Provider: "gemini", Label: "Renamed", InputPerM: 0.6, OutputPerM: 1.6, Enabled: true,
	})
	if err != nil || saved.Label != "Renamed" || !saved.Enabled {
		t.Fatalf("update = %+v %v", saved, err)
	}
	if err := catalog.Delete(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := catalog.Delete(ctx, id); err == nil {
		t.Fatal("second delete should be ErrNotFound")
	}
}
