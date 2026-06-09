-- 036_department_autonomy: per-department autonomy policy for the Operations Fabric.
-- autonomy_policy decides how big/planned department work proceeds:
--   auto_within_budget = proceed if the CFO projection says it fits;
--   user_approval      = always ask the user;
--   both               = auto when it fits AND is under threshold_uusd, else ask.
-- IT/Code departments default to 'both' (set at creation); others 'auto_within_budget'.

ALTER TABLE departments
    ADD COLUMN IF NOT EXISTS autonomy_policy TEXT   NOT NULL DEFAULT 'auto_within_budget',
    ADD COLUMN IF NOT EXISTS threshold_uusd  BIGINT NOT NULL DEFAULT 25000000; -- $25 default, used by 'both'
