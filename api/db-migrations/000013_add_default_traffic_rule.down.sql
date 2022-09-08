WITH dangling_routes AS (
    SELECT
        id
        , default_traffic_rule -> 'routes' as routes
    FROM
        router_versions
),
cleaned_router_versions AS (
    SELECT id, CASE WHEN traffic_rules = 'null' THEN '[]'::jsonb ELSE traffic_rules END AS traffic_rules
    FROM router_versions
),
tbl1 AS (
    SELECT
        id
        , position
        , elem
    FROM cleaned_router_versions, jsonb_array_elements(traffic_rules) WITH ordinality arr(elem, position)
),
tbl2 AS (
    SELECT
        cleaned_router_versions.id
        , position
        , CASE
            WHEN elem IS NOT NULL THEN jsonb_build_object(
                'conditions', elem -> 'conditions',
                'routes', array_to_json(ARRAY(SELECT jsonb_array_elements_text(elem -> 'routes') EXCEPT
                    SELECT jsonb_array_elements_text(dangling_routes.routes)))::jsonb,
                'name', elem -> 'name'
            )
            ELSE NULL
            END AS traffic_rule
    FROM
        cleaned_router_versions
        LEFT JOIN tbl1 ON cleaned_router_versions.id = tbl1.id
        LEFT JOIN dangling_routes ON cleaned_router_versions.id = dangling_routes.id
),
tbl3 AS (
    SELECT
        id,
        json_agg(traffic_rule ORDER BY position) FILTER (where traffic_rule IS NOT NULL) traffic_rules_updated
    FROM tbl2 GROUP BY id
)

UPDATE router_versions AS t
SET traffic_rules = (CASE WHEN traffic_rules_updated IS NOT NULL THEN traffic_rules_updated ELSE '[]'::json END)
FROM tbl3
WHERE tbl3.id = t.id;

ALTER TABLE router_versions DROP column default_traffic_rule;