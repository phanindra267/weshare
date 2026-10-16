-- 000006_contacts.down.sql

DROP TRIGGER IF EXISTS contacts_set_updated_at ON contacts;
DROP TABLE IF EXISTS contacts;
