-- 059: Backfill the org_hierarchy overlay from the authoritative agents.manager_id tree
-- so structural delegation checks have a row for every agent. Idempotent.
INSERT INTO org_hierarchy (tenant_id, agent_id, reports_to, org_level, org_role, can_delegate_to, max_budget_usd)
SELECT a.tenant_id,
       a.id,
       a.manager_id,
       CASE a.org_level WHEN 'l1' THEN 1 WHEN 'l2' THEN 2 WHEN 'l3' THEN 3 ELSE 0 END,
       COALESCE(a.org_role, ''),
       '{}'::uuid[],
       COALESCE(a.monthly_budget_usd, 0)
FROM agents a
WHERE NOT EXISTS (
    SELECT 1 FROM org_hierarchy h
    WHERE h.agent_id = a.id AND h.tenant_id = a.tenant_id
);
