package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

func (l *Lifecycle) Provision(ctx context.Context, spec ProvisionSpec) (*Record, error) {
	mgr, ok := l.managers[spec.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported service type: %s", spec.Type)
	}

	var networkName string
	if err := l.db.QueryRowContext(ctx,
		`select network_name from tenants where id = ?`, spec.TenantID,
	).Scan(&networkName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lookup tenant: %w", err)
	}

	rec, err := mgr.Provision(ctx, provisionerSpec{
		Name:       spec.Name,
		TenantID:   spec.TenantID,
		Network:    networkName,
		Image:      spec.Image,
		Port:       spec.Port,
		DBName:     spec.DBName,
		DBUser:     spec.DBUser,
		DBPassword: spec.DBPassword,
	})
	if err != nil {
		return nil, fmt.Errorf("provision service: %w", err)
	}

	_, err = l.db.ExecContext(ctx,
		`insert into services (id, tenant_id, name, type, state) values (?, ?, ?, ?, ?)`,
		rec.ID, rec.TenantID, rec.Name, string(rec.Type), string(rec.State),
	)
	if err != nil {
		return nil, fmt.Errorf("persist service: %w", err)
	}
	return rec, nil
}

func (l *Lifecycle) Deploy(ctx context.Context, serviceID string, image string, port uint32) (*Record, error) {
	rec, networkName, err := l.getWithNetwork(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	mgr, ok := l.managers[rec.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported service type: %s", rec.Type)
	}
	deployed, err := mgr.Deploy(ctx, provisionerSpec{
		Name:     rec.Name,
		TenantID: rec.TenantID,
		Network:  networkName,
		Image:    image,
		Port:     port,
	}, serviceID)
	if err != nil {
		return nil, fmt.Errorf("deploy service: %w", err)
	}
	return deployed, nil
}

func (l *Lifecycle) Get(ctx context.Context, serviceID string) (*Record, error) {
	rec, _, err := l.getWithNetwork(ctx, serviceID)
	return rec, err
}

func (l *Lifecycle) List(ctx context.Context, tenantID string) ([]*Record, error) {
	rows, err := l.db.QueryContext(ctx,
		`select id, tenant_id, name, type, state from services where tenant_id = ? order by name`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	defer rows.Close()

	var records []*Record
	for rows.Next() {
		rec := &Record{}
		var typ, state string
		if err := rows.Scan(&rec.ID, &rec.TenantID, &rec.Name, &typ, &state); err != nil {
			return nil, fmt.Errorf("scan service: %w", err)
		}
		rec.Type = Type(typ)
		rec.State = State(state)
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate services: %w", err)
	}
	return records, nil
}

func (l *Lifecycle) getWithNetwork(ctx context.Context, serviceID string) (*Record, string, error) {
	rec := &Record{}
	var typ, state, networkName string
	err := l.db.QueryRowContext(ctx,
		`select services.id, services.tenant_id, services.name, services.type, services.state, tenants.network_name
		from services join tenants on tenants.id = services.tenant_id
		where services.id = ?`,
		serviceID,
	).Scan(&rec.ID, &rec.TenantID, &rec.Name, &typ, &state, &networkName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("get service: %w", err)
	}
	rec.Type = Type(typ)
	rec.State = State(state)
	return rec, networkName, nil
}
