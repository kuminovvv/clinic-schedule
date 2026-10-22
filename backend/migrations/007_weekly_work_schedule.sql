-- A change of opening hours never deletes or moves existing appointments.
ALTER TABLE calendar_settings
    ADD COLUMN IF NOT EXISTS work_week JSONB NOT NULL DEFAULT '[
        {"weekday":1,"open":true,"start":"09:00","end":"16:00","breaks":[{"id":"lunch","name":"Обед","start":"13:00","end":"14:00"}]},
        {"weekday":2,"open":true,"start":"09:00","end":"16:00","breaks":[{"id":"lunch","name":"Обед","start":"13:00","end":"14:00"}]},
        {"weekday":3,"open":true,"start":"09:00","end":"16:00","breaks":[{"id":"lunch","name":"Обед","start":"13:00","end":"14:00"}]},
        {"weekday":4,"open":true,"start":"09:00","end":"16:00","breaks":[{"id":"lunch","name":"Обед","start":"13:00","end":"14:00"}]},
        {"weekday":5,"open":true,"start":"09:00","end":"16:00","breaks":[{"id":"lunch","name":"Обед","start":"13:00","end":"14:00"}]},
        {"weekday":6,"open":false,"start":"09:00","end":"16:00","breaks":[]},
        {"weekday":7,"open":false,"start":"09:00","end":"16:00","breaks":[]}
    ]'::jsonb;
