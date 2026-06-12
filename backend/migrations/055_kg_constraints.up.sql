-- 055_kg_constraints: unique keys so knowledge-graph upserts can dedup.
-- Entity identity within a tenant is (name, entity_type) — agent_id is provenance,
-- not part of identity, so the same entity learned by two agents is ONE node.
-- Guarded so a re-run (or a box where the constraint was added manually) is a
-- no-op rather than erroring and marking the migration dirty (Postgres has no
-- ADD CONSTRAINT IF NOT EXISTS).
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'kg_entities_uniq') THEN
    ALTER TABLE kg_entities ADD CONSTRAINT kg_entities_uniq UNIQUE (tenant_id, name, entity_type);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'kg_relationships_uniq') THEN
    ALTER TABLE kg_relationships ADD CONSTRAINT kg_relationships_uniq UNIQUE (tenant_id, source_id, target_id, rel_type);
  END IF;
END $$;
