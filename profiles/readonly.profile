-----------
Definition
-----------

OP1: GET '/{domain}' SEND_NONE READ_MANY 200: List all resources in the collection
[OP2]: GET '/{domain}/{id}' SEND_NONE READ_ONE 200: Get a single resource by identifier

-----------
Execution
-----------

OP1 -> OP2