-- 055_kg_constraints: unique keys so knowledge-graph upserts can dedup.
-- Entity identity within a tenant is (name, entity_type) — agent_id is provenance,
-- not part of identity, so the same entity learned by two agents is ONE node.
ALTER TABLE kg_entities
  ADD CONSTRAINT kg_entities_uniq UNIQUE (tenant_id, name, entity_type);
ALTER TABLE kg_relationships
  ADD CONSTRAINT kg_relationships_uniq UNIQUE (tenant_id, source_id, target_id, rel_type);
