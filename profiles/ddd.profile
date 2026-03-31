-----------
Definition
-----------

OP1: POST '/{domain}' SEND_ONE READ_ONE 201:  Create new resource (and update if no PUT)
[OP2]: PUT '/{domain}' SEND_ONE READ_NONE 200: Update the resource
OP3: GET '/{domain}' SEND_NONE READ_MANY 200: Get a collection of all feasible resources
OP4: GET '/{domain}/{id}' SEND_NONE READ_ONE 200: Get the exact resource identified by `{id}`
OP5: DELETE '/{domain}/{id}' SEND_NONE READ_NONE 204: Delete the resource identified by `{id}`

-----------
Execution
-----------

OP1 -> OP2 -> OP3 -> OP4 -> OP5