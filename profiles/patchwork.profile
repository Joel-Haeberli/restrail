-----------
Definition
-----------

OP1: POST '/{domain}' SEND_ONE READ_ONE 201: Create a new resource
OP2: PATCH '/{domain}/{id}' SEND_ONE READ_ONE 200: Partially update the resource
OP3: GET '/{domain}' SEND_NONE READ_MANY 200: List all resources in the collection
OP4: GET '/{domain}/{id}' SEND_NONE READ_ONE 200: Retrieve a single resource by identifier
OP5: DELETE '/{domain}/{id}' SEND_NONE READ_NONE 204: Remove the resource

-----------
Execution
-----------

OP1 -> OP2 -> OP3 -> OP4 -> OP5