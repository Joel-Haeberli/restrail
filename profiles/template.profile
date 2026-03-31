-----------
Definition
-----------

OP1: METHOD '/PATTERN' CRUD_TYPE_REQUEST CRUD_TYPE_RESPONSE EXPECTED_SUCCESS_HTTP_STATUS: DESCRIPTION [PRECONDITION-DATA]
OP2: METHOD '/PATTERN' CRUD_TYPE_REQUEST CRUD_TYPE_RESPONSE EXPECTED_SUCCESS_HTTP_STATUS: DESCRIPTION [PRECONDITION-DATA]
OP3: METHOD '/PATTERN' CRUD_TYPE_REQUEST CRUD_TYPE_RESPONSE EXPECTED_SUCCESS_HTTP_STATUS: DESCRIPTION [PRECONDITION-DATA]
[OP4]: METHOD '/PATTERN' CRUD_TYPE_REQUEST CRUD_TYPE_RESPONSE EXPECTED_SUCCESS_HTTP_STATUS: DESCRIPTION [PRECONDITION-DATA]

-----------
Execution
-----------

OP1 -> OP2 -> OP3

-----------
Params
-----------

# Optional: explicit path parameter to domain mappings
# Format: PARAM_NAME = DOMAIN_REFERENCE
#   $           = current domain's ID (default behavior when no mapping)
#   domainName  = named domain's ID from registry
#
# When no mapping is specified for a parameter, heuristic matching is used.
#
# Examples:
# number = $
# buildingId = buildings
# floorId = floors