-- Seed 5 local-dev contestants with deterministic UUIDs and API keys.
-- Safe to re-run (ON CONFLICT DO NOTHING).
INSERT INTO contestants (id, name, email, api_key) VALUES
    ('11111111-1111-4111-8111-111111111111', 'Alice',  'alice@example.com',  'key-alice-0001'),
    ('22222222-2222-4222-8222-222222222222', 'Bob',    'bob@example.com',    'key-bob-0002'),
    ('33333333-3333-4333-8333-333333333333', 'Carol',  'carol@example.com',  'key-carol-0003'),
    ('44444444-4444-4444-8444-444444444444', 'Dave',   'dave@example.com',   'key-dave-0004'),
    ('55555555-5555-4555-8555-555555555555', 'Eve',    'eve@example.com',    'key-eve-0005')
ON CONFLICT (id) DO NOTHING;
