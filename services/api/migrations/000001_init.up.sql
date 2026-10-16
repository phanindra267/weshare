-- 000001_init
-- Baseline migration. Establishes a shared helper reused by every table that
-- needs an automatic `updated_at` timestamp. No feature tables are created
-- here yet; those arrive in later phases.

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
