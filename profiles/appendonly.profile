-----------
Definition
-----------

OP1: POST '/{domain}' SEND_ONE READ_ONE 201: Append a new resource to the collection
OP2: GET '/{domain}' SEND_NONE READ_MANY 200: List all resources in the collection
OP3: GET '/{domain}/{id}' SEND_NONE READ_ONE 200: Retrieve the appended resource by identifier

-----------
Execution
-----------

OP1 -> OP2 -> OP3