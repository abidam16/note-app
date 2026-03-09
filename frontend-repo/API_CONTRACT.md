# API Contract Notes for Frontend

## Workspace Creation

### Endpoint
- `POST /api/v1/workspaces`

### Request
```json
{
  "name": "Engineering"
}
```

### Success response
- Status: `201 Created`
- Body contains `workspace` and `membership`.

### Validation adjustments
Workspace names are now validated to avoid duplicate names for the same authenticated user.

If the actor already has a workspace with the same normalized name (trimmed, case-insensitive), the API returns:
- Status: `422 Unprocessable Entity`
- Error code: `validation_failed`
- Error message: `validation failed: workspace name already exists`

Frontend behavior recommendation:
- Show inline form error on workspace name input for this message.
- Keep the modal/page open so user can edit and retry.
