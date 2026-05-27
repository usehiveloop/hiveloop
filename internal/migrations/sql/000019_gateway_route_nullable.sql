-- +goose Up
ALTER TABLE employee_gateway_events ALTER COLUMN route_id DROP NOT NULL;
ALTER TABLE employee_gateway_deliveries ALTER COLUMN route_id DROP NOT NULL;

-- +goose Down
UPDATE employee_gateway_events SET route_id = '00000000-0000-0000-0000-000000000001' WHERE route_id IS NULL;
UPDATE employee_gateway_deliveries SET route_id = '00000000-0000-0000-0000-000000000001' WHERE route_id IS NULL;
ALTER TABLE employee_gateway_events ALTER COLUMN route_id SET NOT NULL;
ALTER TABLE employee_gateway_deliveries ALTER COLUMN route_id SET NOT NULL;
