# KMJG Hub — System Architecture

## Document Status

Status: Draft
Target: KMJG Hub v1

---

## Architecture Goals

The KMJG Hub architecture should support the product and UX requirements defined for v1 while remaining practical for small self-hosted development teams.

The architecture should prioritize:

- Cross-platform desktop support for Linux and Windows
- Self-hosted deployment
- Clear separation between Client and Server
- Real-time collaboration
- Secure authentication and authorization
- Project-level data isolation
- Git and Git provider integration
- Direct and Project-based file sharing
- Partial offline functionality
- Simple deployment and maintenance
- Future extensibility without overengineering v1

Architecture decisions should support the existing Product Requirements and User Experience specifications rather than redefine them.

---

## Desktop Client

KMJG Hub v1 uses Tauri 2 as the desktop application framework.

The Desktop Client targets:

- Windows
- Linux

The architecture should allow macOS support to be added in the future without requiring a redesign of the core system.

Tauri provides the boundary between the web-based user interface and privileged native desktop functionality.

Native desktop responsibilities may include:

- Accessing local Project directories
- Reading local Git repository state
- Launching external development tools
- Opening terminals in the current Project directory
- Interacting with the local filesystem
- Managing local application data and cache
- Performing operating-system-specific integrations

Privileged native operations must not be exposed directly to the web UI without an explicitly defined application interface.

### Native Layer

The native layer of the Desktop Client is implemented in Rust through Tauri.

The Rust layer is responsible for operations that require access to the user's operating system or local development environment.

The frontend should request these operations through controlled Tauri commands or other explicitly defined Tauri interfaces rather than directly accessing privileged system resources.

### Frontend

The KMJG Hub Desktop Client uses React with TypeScript for its user interface.

Vite is used as the frontend development and build tool.

The frontend is responsible for presenting the application experience defined in the UX specification, including:

- Server selection and authentication
- Server Home
- Project Workspace
- Project Chat
- Tasks
- Git information
- Project Files
- Members and presence
- Developer Tools
- Friends and Direct Messages
- Notifications
- User profiles
- Project and Server settings

TypeScript is used to provide explicit types for application data, component interfaces, and communication boundaries.

The frontend must not directly perform privileged operating-system operations.

Operations requiring access to the local filesystem, Git repositories, external applications, or other native system resources must go through the Tauri native layer.

Vite is responsible for the frontend development environment and production frontend build consumed by Tauri.

---

## Server

KMJG Hub uses a self-hosted server architecture.

Each KMJG Hub Server operates independently and is responsible for managing the users, Projects, collaboration data, and services belonging to that Server instance.

The KMJG Hub Server is implemented in Go.

Go is used for the Server because KMJG Hub requires a network-oriented backend capable of handling concurrent connections, real-time collaboration, and self-hosted deployment while remaining relatively simple to distribute and operate.

The Server is responsible for:

- Authentication and user accounts
- Friends and Direct Messages
- Projects and membership
- Roles and permissions
- Project Chat
- Tasks
- Presence and work status
- Notifications
- Git provider integrations
- Project file metadata and storage coordination
- Direct file transfer coordination
- Data persistence
- API services
- Real-time communication

The Server must remain independent from the Desktop Client implementation.

Other clients may communicate with the Server in the future through the same defined application protocols without requiring the Server architecture to depend on Tauri.

### Deployment Model

Each team operates its own KMJG Hub Server instance.

A Server instance has its own:

- Users
- Authentication
- Projects
- Friends
- Direct Messages
- Files
- Configuration
- Database
- Backups

Data from separate KMJG Hub Server instances is not automatically shared or synchronized.

The Server should be deployable without requiring a centralized KMJG-operated cloud service.

---

## Data Persistence

KMJG Hub separates authoritative Server data from local Client data.

### Server Database

The KMJG Hub Server uses PostgreSQL as its primary relational database.

PostgreSQL stores authoritative Server-managed data such as:

- User accounts and authentication data
- User profiles and privacy settings
- Friend relationships and blocks
- Projects and Project membership
- Roles and permissions
- Project invitations
- Project Chat messages
- Direct Messages
- Tasks and task comments
- Notifications
- Git integration metadata
- File metadata
- Soft-deletion and retention metadata
- Server configuration required by application services

The Server database is the source of truth for shared KMJG Hub data.

Files themselves should not be stored directly in PostgreSQL unless a specific future architecture decision requires it. File metadata and references may be stored in the database while file content is handled by the Server's storage layer.

### Client Local Database

The KMJG Hub Desktop Client uses SQLite for local application data and cached Server data.

SQLite may store:

- Saved Server information
- Local Client configuration
- Cached Project information
- Cached messages
- Cached task information
- Latest known member status
- Local Project mappings
- Local Git-related state required by the Client
- Offline-accessible metadata
- Synchronization metadata

Cached Server data must not be treated as authoritative when it conflicts with newer Server state.

Sensitive authentication credentials should not be stored as plain text in SQLite.

The exact authentication token storage mechanism will be defined separately.

### Offline Data

The local database supports the partial offline experience defined in the UX specification.

When disconnected, the Client may present previously cached information while clearly identifying that the information may be stale.

After reconnecting, the Client synchronizes with the Server and updates its local cached state.

---

## Client-Server Communication

The KMJG Hub Desktop Client communicates with the KMJG Hub Server through defined network APIs.

KMJG Hub v1 uses a combination of HTTPS-based APIs and WebSocket connections.

### HTTP API

HTTPS APIs are used for request-response operations that do not require a persistent real-time connection.

These operations may include:

- Authentication
- User and profile information
- Project creation and management
- Project membership and roles
- Invitations
- Task creation and management
- Message history retrieval
- File metadata
- Git integration configuration
- Notification history
- Server and Project settings

The API should use clearly versioned routes so future protocol changes can be introduced without unnecessarily breaking existing Clients.

Example:

    /api/v1/...

All production communication between the Client and Server must use encrypted transport through HTTPS.

### Real-Time Communication

KMJG Hub uses WebSocket connections for Server-to-Client and bidirectional real-time events.

Real-time events may include:

- New Project Chat messages
- New Direct Messages
- Presence changes
- Work status changes
- Current Task changes
- Task assignment requests and updates
- Friend requests
- Project invitations
- Direct File Transfer requests and status
- Notifications
- Git activity events
- Relevant Project activity

The WebSocket connection is associated with the authenticated Server session.

The Server must verify authorization before sending protected real-time data to a connected Client.

Clients must not receive Project events for Projects they are not authorized to access.

### Reconnection

Temporary network interruption must not require the user to restart the application.

When a WebSocket connection is lost, the Client should attempt to reconnect.

After reconnection, the Client must synchronize relevant Server state rather than assuming that no events were missed while disconnected.

The local SQLite cache may be used to support this synchronization process and the offline experience.

WebSocket events should not be treated as the only persistent record of important shared data.

Authoritative persistent data remains managed by the Server and its PostgreSQL database.

---

## Authentication Architecture

Each KMJG Hub Server manages its own user accounts and authentication.

A user must authenticate with the selected Server before accessing that Server's KMJG Hub features.

KMJG Hub v1 supports two primary authentication methods:

- Local KMJG Hub account
- GitHub authentication linked to a Server-local KMJG Hub account

There is no centralized global KMJG Hub account service in v1.

### Local Authentication

Users may register a local account directly with a KMJG Hub Server.

Local registration requires account information such as:

- Username
- Email address
- Password

Users may subsequently log in using their username or email address together with their password.

Passwords must never be stored in plain text.

The Server stores a cryptographically secure password hash using Argon2id.

Password verification is performed by the Server.

### GitHub Authentication

Users may authenticate through GitHub when GitHub authentication is enabled and configured for the Server.

GitHub acts as an external identity provider.

A GitHub identity is associated with a local KMJG Hub user identity belonging to the selected Server.

Using the same GitHub account with multiple KMJG Hub Servers does not create a shared global KMJG Hub account.

Each Server continues to manage its own:

- User identity
- Profile
- Project membership
- Friends
- Direct Messages
- Permissions
- Application data

GitHub authentication must not automatically grant access to a KMJG Hub Project.

Project access remains controlled by KMJG Hub Project membership and invitations.

Similarly, KMJG Hub Project membership does not automatically grant access to a GitHub repository.

Repository permissions remain controlled by the Git provider.

### Authentication Boundary

Successful authentication establishes an authenticated session with the selected KMJG Hub Server.

The authenticated identity is used by both HTTP API requests and the real-time WebSocket connection.

Authentication does not replace authorization.

The Server must independently verify that the authenticated user has permission to perform each protected operation or access protected data.

### Session Management

KMJG Hub v1 uses Server-managed opaque session tokens for authenticated Client sessions.

After successful authentication, the Server generates a cryptographically secure random session token.

The token identifies a Server-side session but should not contain application authorization data that the Client can interpret or modify.

The Server maintains the authoritative session state.

A session record may include information such as:

- User identity
- Session identifier
- Creation time
- Expiration time
- Last activity
- Revocation state
- Client or device information where appropriate

The Client presents its session token when making authenticated HTTP API requests and when establishing an authenticated WebSocket connection.

### Session Security

Session tokens must:

- Be generated using a cryptographically secure random source
- Have sufficient entropy to prevent practical guessing
- Be transmitted only over encrypted connections
- Never be written to application logs
- Be revocable by the Server
- Expire according to Server authentication policy

Possession of a valid session token must not by itself bypass authorization checks.

For every protected operation, the Server must determine the current permissions of the authenticated user.

This ensures that changes such as Project role updates, Project removal, account restrictions, or session revocation can take effect without relying on authorization information embedded in the Client's token.

### Client Token Storage

Authentication tokens must not be stored as plain text in the Client's SQLite database.

The Desktop Client should use an operating-system-supported secure credential storage mechanism where available.

The exact platform-specific credential storage implementation may be selected during implementation.

### Logout and Revocation

Logging out invalidates the relevant Server-side session.

The Server architecture should also support revoking sessions when required, such as when:

- The user explicitly logs out
- A session expires
- The user removes a saved authenticated session
- A Server Administrator invalidates a session for security or administrative reasons

A revoked or expired session must no longer authorize HTTP API requests or WebSocket connections.

---

## Authorization Architecture

KMJG Hub enforces authorization on the Server.

The Desktop Client may hide or disable actions that the current user cannot perform, but Client-side checks are only part of the user experience and must never be treated as a security boundary.

Every protected Server operation must verify the authenticated user's current permissions before performing the operation.

### Server Administrator

Server Administrator authority exists at the Server infrastructure level and is separate from Project roles.

Server Administrators may perform infrastructure and administrative operations required to operate, maintain, recover, and configure the self-hosted KMJG Hub Server.

Server Administrator access does not originate from a Project role.

Being a Project Owner or Admin must not automatically grant Server Administrator privileges.

### Project Roles

KMJG Hub v1 uses three Project roles:

- Owner
- Admin
- Member

Each Project has exactly one Owner at a time.

The Project role determines which Project-level operations a user may perform.

Authorization checks may consider:

- Authenticated user identity
- Project membership
- Current Project role
- Ownership
- Resource relationship
- Server-level restrictions
- Other explicit access rules defined by the product requirements

### Owner Authorization

The Owner has the highest authority within a Project.

Owner-only operations include actions such as:

- Transferring Project ownership
- Deleting the Project
- Managing Admin role changes where required

The Owner must transfer ownership before leaving the Project.

### Ownership Transfer

Ownership transfer is an authoritative Server operation.

Before performing the transfer, the Server must validate that:

- The authenticated user is the current Project Owner.
- The target user is an active member of the Project.
- The target user is not already the current Owner.
- The previous Owner's selected post-transfer role is either Admin or Member.
- The Project is active and eligible for ownership transfer.

The ownership transfer must be performed atomically.

Within the same authoritative operation:

- The selected Project member becomes the new and sole Owner.
- The previous Owner loses the Owner role.
- The previous Owner receives the selected Admin or Member role.
- The previous Owner remains a Project member.

The Server must never commit an ownership transfer that leaves the Project with zero Owners or more than one Owner.

Relevant ownership and role-change events should be published only after the authoritative transaction succeeds.

### Admin Authorization

Admins may perform permitted Project administration operations such as managing regular members, invitations, and supported Project settings.

Admins must not be able to:

- Delete the Project
- Transfer ownership
- Remove or demote the Owner
- Promote or demote other Admins

### Member Authorization

Members may use normal Project collaboration functionality but cannot perform administrative operations reserved for Owner or Admin roles.

### Resource-Level Authorization

Project role alone is not sufficient for every access decision.

The Server must also enforce resource-specific rules.

Examples include:

- Direct Messages may only be accessed by their participants.
- Project Chat may only be accessed by authorized Project members.
- Tasks may only be accessed within Projects where the user has access.
- Project files may only be accessed by authorized Project members.
- Profile fields must respect the owner's configured privacy audience.
- Blocking rules must be enforced for Direct Messages and Friend Requests.
- Repository access must continue to respect the external Git provider's permissions.

No Project role grants permission to read private Direct Messages between other users.

### Authorization and Open Source Clients

The Server must not trust the Desktop Client to enforce permissions.

A modified, outdated, or third-party Client must not be able to bypass Server authorization by directly calling an API or sending a WebSocket event.

All security-sensitive authorization decisions are enforced by the Go Server.

---

## Git Provider Integration

Git integration is a first-class capability of KMJG Hub, but Git provider authorization remains separate from KMJG Hub authentication and Project authorization.

KMJG Hub v1 primarily integrates with GitHub while keeping the architecture capable of supporting additional Git providers in the future.

The architecture must not assume that every Git repository is hosted on GitHub.

### GitHub Identity and Repository Access

GitHub authentication and GitHub repository integration are separate concerns.

GitHub authentication may be used to identify a user and associate their GitHub identity with their Server-local KMJG Hub account.

Repository integration may require additional GitHub authorization depending on the operation being performed.

KMJG Hub should request only the GitHub permissions required for the features the user chooses to use.

A user should not be required to grant unnecessary repository management permissions simply to authenticate with KMJG Hub.

### Repository Connection

A KMJG Hub Project may have zero or one connected Git repository in v1.

A repository may be connected:

- During Project creation
- Later through Project settings

Supported v1 connection flows include:

- Creating a new GitHub repository
- Selecting an existing repository from the user's connected GitHub account
- Entering a repository URL
- Leaving the Project without a repository and connecting one later

Repository metadata stored by KMJG Hub should identify the Git provider independently from the repository itself.

Conceptually, a repository connection may include information such as:

- Git provider
- Repository identifier
- Repository URL
- Repository owner or namespace
- Repository name
- Default branch
- Connection metadata

The data model should avoid assuming that all repository URLs use github.com.

### Project Membership and Repository Permission

KMJG Hub Project membership and Git repository permission are independent authorization systems.

Joining a KMJG Hub Project does not automatically grant repository access.

Similarly, repository access does not automatically grant membership in a KMJG Hub Project.

When possible, KMJG Hub may display whether a Project member currently has access to the connected repository.

### Repository Invitations

When a Project member does not have repository access, an authorized user may request that KMJG Hub invite the member through the configured Git provider.

KMJG Hub must use the provider's supported permission model and APIs.

The Server must verify that:

- The KMJG user is authorized to perform the KMJG Project operation
- The connected Git provider identity has sufficient provider-side permission
- The requested repository permission is supported by the provider

KMJG Hub must never attempt to bypass Git provider authorization.

### Repository Access Removal

When removing a member from a KMJG Hub Project, the authorized user may optionally request removal of that member's repository access where supported by the Git provider.

Project removal and repository access removal are separate operations.

Failure to remove repository access must not undo a successful KMJG Project membership removal.

The Client must clearly report partial success when Project membership is removed but external repository access remains.

The same separation applies when a user voluntarily leaves a Project and requests removal of their own repository access.

### Provider Abstraction

GitHub is the primary Git provider integration for KMJG Hub v1.

Provider-specific behavior should be isolated behind a Git provider integration boundary rather than spread throughout unrelated application services.

This allows future providers such as GitLab or self-hosted Git services to be introduced without redesigning the core Project model.

Provider-specific capabilities may differ, so KMJG Hub should not assume that every provider supports identical repository permissions or collaboration operations.

---

## File Architecture

KMJG Hub separates persistent Project file sharing from Direct File Transfer.

These two file flows have different storage and lifecycle requirements.

### Project Chat Attachments

Files attached to Project Chat messages are persistent Server-managed Project data.

When an authorized member sends a Project Chat attachment:

1. The Client uploads the file to the KMJG Hub Server.
2. The Server validates the upload against configured Server and Project limits.
3. The Server stores the file in its configured file storage.
4. File metadata is stored in PostgreSQL.
5. The attachment is associated with its Project Chat message.
6. Authorized Project members may download the file later.

File content should be stored in the Server's file storage layer rather than directly inside PostgreSQL.

PostgreSQL stores metadata and references required to locate and authorize access to the file.

The Server must verify Project authorization before allowing an upload or download.

### File Storage Layer

KMJG Hub treats file storage as a separate architectural responsibility from relational data persistence.

For v1, a self-hosted Server may use Server-managed filesystem storage for persistent Project files.

The storage architecture should avoid coupling application services directly to specific filesystem paths.

This allows alternative storage backends to be introduced in the future without changing the Project Chat or authorization model.

Server Administrators may configure limits such as:

- Maximum individual upload size
- Maximum Project storage usage

The Client should communicate relevant configured limits and upload failures clearly to the user.

### Attachment Lifecycle

A Project Chat attachment is associated with the lifecycle of its message.

When the related message is soft-deleted, the attachment must no longer be available through normal Client access.

The attachment remains subject to the configured soft-deletion retention period before removal from normal Server storage.

Backup retention is independent from normal application retention.

Deleted file content may therefore remain in Server backups until those backups expire or are removed according to Server Administrator configuration.

### Direct File Transfer

Direct File Transfer is separate from persistent Project file storage.

A Direct File Transfer begins with a transfer request containing metadata such as:

- Sender
- Recipient
- File name
- File size

The recipient must explicitly Accept or Decline the request.

Before the recipient accepts, the file content must not be uploaded to KMJG Hub Server storage.

### Direct Transfer Relay

KMJG Hub v1 uses the Server as a transfer relay after the recipient accepts the Direct File Transfer request.

After acceptance:

1. The Server authorizes the sender and recipient.
2. The Client begins sending the file through the Server.
3. The Server relays the transfer to the recipient.
4. Transfer progress is reported to the participating Clients.
5. Either participant may cancel the transfer where applicable.

Direct File Transfer content is not a persistent Project File.

The Server should avoid retaining completed Direct File Transfer content as persistent application storage.

Temporary buffering required to perform the transfer must be limited to the transfer lifecycle and cleaned up when it is no longer required.

An interrupted transfer may fail and require the user to start a new transfer.

Transfer resume is not required for v1.

### Future Peer-to-Peer Transfer

Peer-to-peer Direct File Transfer is not required for KMJG Hub v1.

The Direct File Transfer architecture should keep the user-facing transfer model separate from the underlying transport mechanism so that peer-to-peer transfer may be investigated in a future version.

A future implementation may use peer-to-peer transport with Server relay fallback without requiring the Direct File Transfer user experience to be redesigned.

### Persistent Server Storage

KMJG Hub v1 uses the Server's local filesystem as the default persistent file storage backend.

This is intended to keep self-hosted deployment simple for small teams and allows a KMJG Hub Server to operate using storage available directly on the host machine.

Persistent file content may include:

- Project Chat attachments
- Other Server-managed Project files introduced by supported v1 features

Application services must not depend directly on hardcoded filesystem paths.

Instead, persistent file operations should pass through a defined storage abstraction.

Conceptually:

    Application Services
            |
            v
      Storage Interface
            |
            v
    Local Filesystem Storage

The storage interface is responsible for operations such as:

- Store file
- Open file for download
- Delete file
- Check file existence
- Determine file size
- Track storage usage where required

PostgreSQL stores file metadata and storage references while the storage backend stores the actual file content.

### Storage Location

The Server Administrator must be able to configure the directory used for persistent KMJG Hub file storage.

The Server must not assume a user-specific or development-machine-specific path.

A deployment may, for example, mount persistent storage into a dedicated Server data directory.

The exact host path is deployment-specific and must not be embedded into application data unnecessarily.

### Storage Identifiers

Application data should reference stored files using internal storage identifiers rather than exposing or depending on absolute host filesystem paths.

Clients must not be allowed to request arbitrary filesystem paths from the Server.

Every file request must resolve through Server-controlled storage logic and authorization checks.

### Future Storage Backends

The storage abstraction should allow additional persistent storage backends to be introduced in the future.

Potential future backends may include S3-compatible object storage or other self-hosted storage systems.

Support for these backends is not required for KMJG Hub v1.

---

## Deployment Architecture

KMJG Hub Server is designed to run on infrastructure controlled by the Server Administrator.

The Server application must not depend on a specific hosting provider or deployment environment.

A KMJG Hub Server may run on environments such as:

- A Linux desktop or workstation
- A dedicated home or office server
- A virtual machine
- A VPS
- Other compatible self-hosted infrastructure

Moving the Server to different infrastructure should not require redesigning the KMJG Hub application architecture.

### Containerized Deployment

The recommended KMJG Hub v1 deployment uses containers managed through Docker Compose.

A basic deployment may contain:

    KMJG Hub Deployment
    |
    +-- kmjg-server
    +-- postgres
    +-- persistent-data
        +-- file-storage
        +-- database-data

Containerized deployment provides a consistent Server environment across development machines, office servers, and future hosted infrastructure.

Persistent application data must exist outside the disposable container filesystem through persistent volumes or configured host storage.

Restarting or replacing an application container must not delete Server data.

### Network Exposure

KMJG Hub Server does not require one specific method of Internet exposure.

The Server provides its application service on a configured local network interface and port.

Deployment infrastructure determines how that service becomes reachable by Clients.

Supported deployment patterns may include:

- Local network access
- Cloudflare Tunnel
- Reverse proxy with HTTPS
- Other secure network infrastructure

Network exposure is therefore a deployment concern rather than a dependency of the KMJG Hub application architecture.

### Cloudflare Tunnel

A Server Administrator may expose KMJG Hub through Cloudflare Tunnel.

This allows a deployment to make the Server available through a configured domain without requiring direct inbound port forwarding on the host network.

Cloudflare Tunnel is optional and is not a required component of KMJG Hub Server.

The Server must remain capable of operating without Cloudflare services.

### Domain and Server Address

KMJG Hub Clients connect to a Server using a Server address configured by the user.

For example:

    https://hub.example.com

The public domain or network address represents the deployment endpoint and must not be hardcoded into KMJG Hub Server.

A Server Administrator may move a KMJG Hub deployment to different infrastructure and configure the appropriate network endpoint without changing the core application.

### Transport Security

Connections exposed over untrusted networks must use encrypted transport.

HTTPS is used for HTTP API communication and secure WebSocket transport is used for real-time communication.

TLS termination may be provided by deployment infrastructure such as a secure tunnel or reverse proxy.

The KMJG Hub application must not assume that TLS termination is performed by one specific provider.

---

## Backup and Recovery Architecture

KMJG Hub treats backups as a Server administration responsibility that is separate from normal application soft-deletion and retention behavior.

A Server backup should contain the information required to recover a KMJG Hub Server after data loss or infrastructure failure.

### Backup Scope

A complete Server backup should include:

- PostgreSQL data
- Persistent Project file storage
- Server configuration required for recovery
- Storage metadata required to reconnect database records with stored files

Temporary data such as active Direct File Transfer buffers does not need to be included in backups.

Client-side SQLite caches are not authoritative Server data and are not part of Server backups.

### Backup Retention

Backup retention is configured by the Server Administrator.

Server configuration may define policies such as:

- Whether automatic backups are enabled
- Backup frequency
- Backup retention duration
- Maximum number of retained backups
- Backup storage location

The exact default values may be selected during implementation.

### Backup Storage

Backups must not be assumed to exist only inside the KMJG Hub application container.

A deployment may store backups in:

- A configured host directory
- Another local disk
- Network-attached storage
- An external backup system
- Other Server Administrator controlled storage

For meaningful protection against host disk failure, Server Administrators should be able to place backups on storage separate from the primary KMJG Hub data.

### Soft Deletion and Backups

Application soft-deletion retention and backup retention are independent.

For example, a Project or message may be permanently removed from normal KMJG Hub Server storage after its application retention period while older backups may still contain that data until the corresponding backups expire or are deleted.

KMJG Hub documentation and administrative interfaces must communicate this distinction clearly.

### Recovery

Recovery must restore a consistent relationship between:

- PostgreSQL data
- Persistent file storage
- Required Server configuration

The recovery process should not depend on the original physical machine.

A valid backup should therefore be usable when migrating or recovering KMJG Hub onto compatible replacement infrastructure.

Recovery procedures and backup formats will be defined in more detail during implementation and deployment design.

## Project Invitation Architecture

KMJG Hub supports Project invitations as Server-managed resources.

Project invitations are separate from Git repository invitations. Joining a KMJG Hub Project does not automatically grant access to the connected Git repository.

Only users authorized by the Project role rules may create or manage Project invitations.

New users joining through a Project invitation enter the Project with the Member role by default.

### Direct Invitations

A Direct Invitation targets an existing user on the same KMJG Hub Server.

The recipient must explicitly Accept or Decline the invitation before becoming a Project member.

The sender may select one of the supported expiration periods:

- 1 hour
- 1 day
- 7 days
- 30 days
- Never

A pending Direct Invitation may be cancelled only by the inviter before it is accepted.

The Server stores the authoritative invitation state and validates that the invitation:

- Exists
- Has not expired
- Has not been cancelled
- Has not already been accepted or declined
- Targets the authenticated recipient
- Belongs to an active Project

Accepting a valid Direct Invitation adds the recipient to the Project as a Member.

Declining the invitation does not add the recipient to the Project.

### Invite Links and Codes

An authorized Project user may create an Invite Link or Invite Code.

Each Invite Link or Code may define:

- An expiration time
- A maximum number of uses, or unlimited uses

An authorized Project user may revoke an active Invite Link or Code.

When an authenticated user intentionally uses a valid Invite Link or Code, the user joins the Project immediately as a Member without requiring additional Project approval.

If the user is not authenticated, the Client requires the user to register or log in to the same KMJG Hub Server before completing the join operation.

After authentication, the Server must validate the invitation again before adding the user to the Project.

### Invite Validation

Invitation validation is performed by the Server.

The Server must reject an invitation when applicable conditions indicate that it is:

- Expired
- Revoked
- Cancelled
- Already consumed where reuse is not permitted
- Over its configured maximum use count
- Associated with a deleted or unavailable Project
- Used by an unauthorized recipient in the case of a Direct Invitation

Invitation state must not rely on Client-side validation.

Use counts and membership changes must be updated safely so concurrent requests cannot exceed the configured invitation limit.

### Membership and Repository Access

Successful Project invitation acceptance grants KMJG Hub Project membership only.

It does not imply Git repository access.

If the Project has a connected repository and the new member does not have repository access, repository permission may be managed separately through the configured Git provider integration according to the provider's supported permission model.

KMJG Hub must never bypass the Git provider's authorization rules.

### Real-Time Updates

Invitation changes may be delivered to connected Clients through the authenticated WebSocket connection.

Relevant events may include:

- Direct Invitation created
- Direct Invitation accepted
- Direct Invitation declined
- Direct Invitation cancelled
- Invite Link or Code revoked
- Project membership created from an invitation

Persistent invitation state remains authoritative on the Server even when a real-time event is missed.

### Notifications

Direct Project Invitations generate a notification for the intended recipient.

Invitation-related notifications reference the Server-managed invitation resource rather than granting access themselves.

Opening an old notification must not allow an expired, cancelled, revoked, or otherwise invalid invitation to be used.

### Security

Invitation identifiers, links, and codes must be generated so they cannot be practically guessed.

The Server must perform authorization and invitation-state validation for every operation that creates, modifies, accepts, or consumes an invitation.

Possession of an invalid or expired Invite Link or Code must not grant Project access.

---

## Messaging and Real-Time Data Architecture

KMJG Hub separates persistent collaboration data from transient real-time delivery.

WebSocket connections provide real-time delivery but are not the authoritative storage mechanism for persistent application data.

### Persistent Messages

Project Chat messages and Direct Messages are persistent Server-managed data.

When the Server receives a new persistent message, the general processing flow is:

1. Authenticate the sender.
2. Verify authorization for the target conversation.
3. Validate the message.
4. Persist the authoritative message state.
5. Acknowledge successful creation.
6. Publish the relevant real-time event to authorized connected Clients.

The Server must not depend on every recipient being online when a message is created.

An offline recipient can retrieve missing persistent messages after reconnecting.

### Project Chat

Project Chat messages belong to a Project.

Before allowing a user to send or retrieve Project Chat messages, the Server verifies that the user is an authorized member of that Project.

Project Chat events are delivered only to Clients authorized to access the Project.

Configured Git and system activity may also appear in the Project Chat timeline while remaining distinguishable from normal user messages.

### Direct Messages

Direct Messages belong to a private conversation between authorized participants.

The Server must enforce Direct Message access independently from Project roles.

Project Owner, Project Admin, and Project Member roles do not grant access to Direct Messages between other users.

Direct Message eligibility follows the product rules for:

- Friends
- Shared Project membership
- Blocking

### Persistent and Transient Events

Not every real-time event requires permanent storage in the same way.

Persistent application events include data whose authoritative state must remain available after disconnection.

Examples include:

- Project Chat messages
- Direct Messages
- Tasks and task updates
- Friend Requests
- Project invitations
- Notifications that require history
- Git activity recorded by the Project

Transient real-time events may include:

- Online presence changes
- Connection state
- Temporary transfer progress
- Typing indicators if introduced
- Other short-lived activity that does not require permanent history

Transient events may be delivered through WebSocket without being retained as permanent application records unless another requirement requires persistence.

### Reconnection and Missed Data

Clients must assume that real-time events may be missed during disconnection.

After reconnecting, the Client synchronizes authoritative state through the Server rather than relying only on WebSocket event history.

The synchronization mechanism should allow the Client to determine which relevant persistent data changed while it was disconnected.

The exact synchronization strategy and cursor or versioning mechanism will be selected during implementation design.

### Event Authorization

The Server determines which connected Clients may receive each real-time event.

A Client must never be trusted to subscribe to arbitrary protected resources simply by knowing a Project, conversation, user, or resource identifier.

Authorization must be verified by the Server before protected events are delivered.

---

## Task Architecture

Tasks are persistent Project-scoped collaboration data managed authoritatively by the Server.

All Project members may create Tasks according to the product requirements.

### Task Model

A Task may contain:

- Title
- Description
- Status
- Assignee
- Creator
- Optional Due Date

KMJG Hub v1 supports the following Task statuses:

- To Do
- In Progress
- Done

### Task Assignment

A Project member may assign a Task to themselves without requiring an additional acceptance step.

When one Project member assigns a Task to another member, the assignment requires an explicit response from the recipient.

The recipient may:

- Accept the assignment
- Decline the assignment

The Server must persist the authoritative assignment state and validate that the relevant users are authorized members of the Project.

Task assignment requests and responses may be delivered through the real-time communication system while remaining recoverable from persistent Server state when required.

### Current Task

A user may have multiple assigned Tasks but only one Current Task at a time.

Selecting a To Do Task as the Current Task may transition that Task to In Progress according to the product rules.

Tasks are moved to Done through an explicit user action.

### Task Comments

Authorized Project members may read and post comments on Project Tasks.

Task Comments are persistent Server-managed Project data and follow normal Project authorization rules.

KMJG Hub v1 does not require file attachments inside Task Comments.

## Friends and Direct Messages Architecture

Friends and Direct Messages are Server-level features in KMJG Hub and are not owned by any individual Project.

Friendships, Friend Requests, Direct Message relationships, and blocking state are authoritative Server data stored independently from Project membership.

Leaving or being removed from a Project must not automatically remove an existing friendship.

### Friend Requests

Users may send Friend Requests to other users on the same KMJG Hub Server.

A Friend Request has a Server-managed state such as:

- Pending
- Accepted
- Declined

The recipient must explicitly Accept or Decline a pending Friend Request before a friendship is created.

Friendship is not required for users to belong to the same Project.

The Server must enforce blocking rules before allowing a Friend Request to be created or accepted.

### Direct Message Authorization

Direct Message access is determined by the Server.

A user may start or continue a Direct Message conversation with another user when at least one permitted relationship exists, including:

- The users are Friends.
- The users are members of at least one shared active Project.

Users who share a Project may therefore communicate through Direct Messages without first becoming Friends.

If two users no longer share an active Project, the Project relationship alone must no longer authorize Direct Messaging.

They may continue Direct Messaging only when another permitted relationship exists, such as an accepted friendship.

A user who is neither a Friend nor a member of a shared active Project with another user must not be permitted to start or continue Direct Messaging with that user.

The Server must re-evaluate Direct Message authorization when relevant relationships change, including:

- Project membership changes
- Friendship changes
- Blocking changes

Existing Direct Message history must not itself grant permission to continue sending messages.

### Direct Message Privacy

Direct Messages are private Server-level conversations between their participants.

Project roles such as Owner, Admin, and Member do not grant access to Direct Messages belonging to other users.

Being the Owner or Admin of a Project shared by two users must not allow that Owner or Admin to inspect their Direct Messages.

The trusted Server Administrator remains an infrastructure authority according to the Server privacy, administration, recovery, and retention model defined elsewhere in this architecture.

### Blocking

Blocking is a Server-level personal communication control.

When User A blocks User B, the Server must enforce the block for personal communication between those users.

Blocking:

- Prevents Direct Messages between the two users.
- Prevents Friend Requests between the two users.
- Removes an existing friendship between the two users.
- Does not remove either user from a Project.
- Does not change Project roles or Project permissions.

The blocked user may be explicitly informed that they have been blocked where required by the Client experience.

Blocking must not be treated as a Project authorization rule.

If both users remain members of the same Project, they may continue to see and interact with shared Project resources according to their Project permissions, including:

- Project Chat
- Tasks
- Git activity
- Project files
- Other authorized Project information

Blocking another user must therefore never be used to hide Project information that the blocked user is otherwise authorized to access.

### Unblocking

Unblocking removes the personal communication block but does not automatically restore the previous friendship.

If the users want to become Friends again, a new Friend Request must be created and accepted.

After unblocking, Direct Messaging becomes available only when the normal Direct Message authorization rules are satisfied.

### Relationship Independence

The Server must keep the following concepts separate:

    Server Account
        |
        +-- Friendship / Friend Requests
        |
        +-- Blocking
        |
        +-- Direct Messages
        |
        +-- Project Membership
                |
                +-- Owner / Admin / Member

A friendship does not grant Project access.

Project membership does not automatically create a friendship.

Blocking does not revoke Project access.

Project roles do not grant access to private Direct Messages.

These boundaries must be enforced by the Server rather than relying on Client behavior.

---

## Presence and Work Status Architecture

KMJG Hub separates connection presence from development work status.

Presence, Work Status, Current Task, and Current Branch represent different types of user state and must not be treated as a single status value.

### Presence

Online presence is determined automatically by the Server based on active authenticated Client connections.

A user may be considered Online while at least one valid Client connection for that user remains active.

The Server must not depend on a permanently stored `online` boolean as the authoritative source of presence.

When a Client disconnects normally, the Server updates the user's connection state.

Unexpected disconnections, application crashes, device shutdowns, and network failures must also eventually cause the user to become Offline.

The real-time connection mechanism may use heartbeat, timeout, or equivalent connection-liveness detection to determine when a connection is no longer active.

The exact heartbeat interval and timeout values will be selected during implementation.

Presence changes are distributed to authorized Clients through the real-time communication system.

### Multiple Client Connections

A user may have more than one active Client connection.

The Server should determine user-level presence from the user's active authenticated connections rather than assuming one connection per user.

Disconnecting one Client must not make the user Offline if another valid Client connection remains active.

### Work Status

Work Status is separate from Online presence.

A user may be:

- Online and Working
- Online and Not Working
- Offline

Working status may be determined through a combination of supported automatic Project-related activity and explicit user control.

Users may manually Start or Stop their Working status.

Manual user control must take precedence where required by the UX.

The exact automatic activity detection rules will be selected during implementation.

### Current Project Context

Work-related state is associated with the relevant KMJG Hub Project context.

The Server may receive Project context updates from an authenticated Client while the user is actively working with a Project.

A Client must not report work state for a Project the authenticated user is not authorized to access.

### Current Task

A user may have multiple assigned Tasks but only one Current Task at a time.

Current Task state is persistent Server-managed Project data.

When a Current Task changes, the Server validates the user and Project relationship, updates the authoritative state, and distributes the relevant update to authorized connected Clients.

Selecting a To Do Task as the Current Task may also transition the Task to In Progress according to the product rules.

### Current Branch

Current Branch information originates from the user's local development environment.

The Desktop Client may detect the active Git branch through the Tauri native layer when a local repository is associated with the current Project.

The Client may report the relevant branch state to the Server so authorized Project members can see it according to profile privacy rules.

The Server must treat Client-reported local Git state as informational rather than as proof of repository authorization or repository contents.

Current Branch information must not grant Git provider permissions.

### Offline State

When a user becomes Offline, KMJG Hub must not continue presenting Working, Current Task, or Current Branch as current live activity.

Persistent information such as task assignments remains available through the normal Project data model.

Previously reported live work information may be cached for synchronization or internal purposes but must not be presented to other users as current live state while the user is Offline.

### Privacy

Presence and work-related information must respect the user's configured profile privacy settings.

The Server determines whether another user is authorized to receive or retrieve each protected profile field.

Client-side hiding alone is not sufficient to enforce profile privacy.

---

## Notification Architecture

Notifications are a Server-level capability in KMJG Hub.

A user may receive notifications from multiple Projects and Server-level features regardless of which Project is currently open in the Desktop Client.

### Notification Sources

Notifications may be generated by events such as:

- Direct Messages
- Direct File Transfer requests
- Friend Requests
- Project invitations
- Task assignments
- Relevant task comments
- Project role changes
- Configured Git push activity
- Other important Server or Project events defined by the product requirements

Routine Project Chat messages do not require individual notifications by default.

### Notification Generation

Application services generate notifications when an event requires a user's attention.

Conceptually:

    Application Event
           |
           v
    Notification Service
           |
           +----> PostgreSQL
           |
           +----> Real-Time Delivery
                         |
                         v
                   Connected Client

The Notification Service determines the intended recipients based on the event, Project configuration, user relationships, permissions, and notification rules.

### Persistent Notifications

Notifications that require history are persisted by the Server.

A persistent notification may contain information such as:

- Recipient
- Notification type
- Related Server or Project context
- Related resource identifier
- Creation time
- Read or unread state

The notification should reference application resources rather than duplicate unnecessary copies of the underlying data.

For example, a task notification may reference the relevant Task so the Client can navigate directly to it.

### Real-Time Delivery

When the recipient is currently connected, newly created notifications may also be delivered immediately through the authenticated WebSocket connection.

Real-time delivery does not replace persistence.

If the user is offline when a persistent notification is created, the notification remains available when the user reconnects.

### Notification Navigation

Notifications should contain sufficient context for the Client to navigate to the relevant application location.

Examples include:

- Direct Message conversation
- Project invitation
- Task
- Task comment
- Project member or role context
- Git activity
- Direct File Transfer request when still active

The Client must verify that the referenced resource is still available when the user opens the notification.

The Server must perform normal authorization checks when the resource is requested.

### Git Push Notifications

Git push notification behavior is controlled by Project configuration.

The Project Owner may configure notification behavior such as:

- Whether Git activity is posted to Project Chat
- Whether members receive notifications
- Which members receive notifications
- Whether all branches or selected branches trigger configured behavior

Git activity may remain visible as Project system activity even when a particular user does not receive an attention notification.

### Read State

Persistent notifications may have read and unread state managed by the Server.

Changes to notification state may be synchronized across multiple authenticated Clients belonging to the same user.

### Notification Privacy

A notification must not expose protected information to a recipient who is not authorized to access the underlying resource.

Removing a user's Project access must prevent subsequent access to protected Project resources even if an older notification referencing that resource still exists.

---

## Soft Deletion and Data Retention

KMJG Hub separates normal deletion from permanent data removal.

Where the product requirements define a recovery or retention period, deleting a resource first places it into a soft-deleted state rather than immediately destroying the underlying data.

### Soft-Deleted Data

A soft-deleted resource is treated as unavailable through normal application functionality.

The Server records deletion metadata required to enforce the retention lifecycle.

This metadata may include:

- Deletion time
- Scheduled permanent deletion time
- User responsible for the deletion where relevant
- Recovery information where recovery is supported

Normal Client APIs must not expose soft-deleted data as active application data.

### Project Deletion

Deleting a Project places the Project into a soft-deleted state for 30 days.

During this period:

- The Project is unavailable through normal Project access.
- Normal collaboration activity is disabled.
- The user who was the Project Owner at the time of deletion may restore the Project through the supported recovery experience.
- A Server Administrator may perform supported Server-side recovery.
- The connected external Git repository is not deleted.

After the 30-day retention period expires, the Server may permanently remove the Project's normal application data according to the cleanup process.

Project deletion must never automatically delete the associated external Git repository.

### Message Deletion

Message deletion authorization is enforced by the Server.

For user-generated messages, the Server must verify the deletion request according to the message context.

For Project Chat:

- A user may delete a Project Chat message they originally sent.
- The Project Owner or an Admin may delete user-generated Project Chat messages for moderation purposes.
- A regular Member must not delete another member's Project Chat message.

For Direct Messages:

- A user may delete only Direct Messages they originally sent.
- Project roles such as Owner or Admin do not grant permission to delete another user's Direct Messages.
- A user must not delete a Direct Message originally sent by the other participant.

Configured Git activity and Server-generated system activity are not ordinary user-authored messages and must not be deletable through the standard user message deletion operation.

The Server must perform the authorization check before marking the message as deleted.

During the retention period, a Server Administrator may recover a deleted message when required for supported administration or recovery purposes.

Normal Project members, including Project Owners and Admins, must not be able to inspect deleted message contents through the normal Client.

Deleted messages are hidden from normal Client access while retained according to the configured application deletion lifecycle.

For KMJG Hub v1, deleted messages remain soft-deleted for 30 days before removal from normal application storage.

Deletion state is authoritative on the Server and must not rely only on the Client hiding the message.

### Attachment Deletion

Project Chat attachments follow the lifecycle of their associated message.

When the message is soft-deleted:

- The attachment becomes unavailable through normal Client access.
- The attachment remains retained during the applicable soft-deletion period.
- File metadata records the deletion lifecycle.
- Persistent file content may remain in the Server storage backend until permanent cleanup occurs.

After the retention period expires, the normal storage copy may be permanently removed.

### Permanent Cleanup

Permanent deletion should be performed by a Server-managed cleanup process.

The cleanup process identifies resources whose retention period has expired and removes data that is no longer required by normal application storage.

Cleanup must preserve data integrity between related PostgreSQL records and persistent file storage.

The cleanup process must be safe to retry if interrupted.

The exact scheduling mechanism for cleanup will be selected during implementation.

### Recovery

Only resource types with a defined recovery experience need to be recoverable through the normal Client.

For KMJG Hub v1, deleted Projects support the defined Recently Deleted Projects recovery flow for the user who was the Project Owner at the time of deletion during the retention period.

A successful normal Client recovery restores that user as the Project Owner unless an exceptional Server Administrator recovery procedure explicitly resolves ownership differently.

Soft deletion must not be treated as an authorization bypass.

A user must still satisfy the applicable recovery permissions before a resource can be restored.

### Backup Independence

Permanent removal from normal application storage does not imply immediate removal from historical Server backups.

Backup retention is managed independently by the Server Administrator.

Therefore:

    Active Data
        |
        v
    Soft Deleted
        |
        | retention period
        v
    Permanently Removed
    From Normal Storage

may exist independently from:

    Historical Backups
        |
        v
    Removed according to
    Backup Retention Policy

Administrative documentation must clearly communicate this distinction.

### Retention Configuration

Retention periods explicitly required by the KMJG Hub v1 product behavior must be preserved by the Server.

Other retention policies may be configurable by the Server Administrator where permitted by the product requirements.

Server configuration must not silently weaken a product-level recovery guarantee.

---

## Offline and Synchronization Architecture

KMJG Hub v1 provides partial offline functionality rather than a fully offline collaborative editing system.

The Server remains the authoritative source of shared application data.

The Desktop Client uses its local SQLite database to retain selected Server data for offline viewing and faster application startup.

### Cached Data

The Client may cache data such as:

- Server information
- Project information
- Previously loaded Project Chat messages
- Previously loaded Direct Messages
- Tasks
- Project members
- Latest known Project activity
- Git-related metadata
- Notifications
- Profile information permitted by privacy rules

Cached information must be associated with the Server from which it originated.

Data from separate self-hosted Servers must remain isolated in local Client storage.

### Offline Mode

When the Client loses connectivity, it remains in the current application context where possible.

The Client clearly indicates that it is offline and that displayed Server data may be cached or stale.

Operations that require authoritative Server interaction must be disabled or reported as unavailable while offline.

These operations include actions such as:

- Sending Project Chat messages
- Sending Direct Messages
- Creating or modifying shared Tasks
- Accepting Project invitations
- Changing Project membership or roles
- Changing Server-managed settings
- Uploading Project files
- Starting network-dependent Direct File Transfers
- Performing Git provider operations

KMJG Hub v1 does not require queuing these collaborative mutations for automatic submission after reconnecting.

This avoids introducing hidden delayed actions and complex conflict resolution into the v1 offline model.

### Local Operations

Operations that depend only on the user's local development environment may remain available while the Server is offline where practical.

Examples may include:

- Opening the local Project directory
- Launching Terminal
- Launching VS Code
- Launching configured developer tools
- Inspecting local Git state

Local operations must not be presented as synchronized Server activity until communication with the Server is restored.

### Reconnection

When connectivity returns, the Client re-establishes its authenticated Server connection.

The reconnection flow includes:

1. Validate or re-establish the authenticated session.
2. Re-establish the real-time WebSocket connection.
3. Determine which authoritative Server data has changed.
4. Retrieve relevant missed or updated persistent data.
5. Update the local SQLite cache.
6. Resume normal real-time operation.

The Client should display a synchronization state while this process is occurring where appropriate.

### Synchronization Model

KMJG Hub uses a Server-authoritative synchronization model.

The Client must not resolve conflicts by assuming that cached state is newer or more authoritative than Server state.

For shared Server-managed data:

    PostgreSQL / Server State
              |
              v
        Authoritative

    SQLite Client Cache
              |
              v
         Derived Copy

The exact incremental synchronization mechanism may use resource versions, cursors, sequence identifiers, timestamps, or another suitable strategy selected during implementation.

The architecture should avoid requiring the Client to re-download all Server data after every temporary disconnection.

### Missed Real-Time Events

WebSocket events may be missed while the Client is disconnected.

The Client must therefore use persistent Server state to recover relevant changes after reconnecting.

WebSocket event delivery is an optimization for real-time experience and is not the only mechanism by which the Client learns authoritative state.

### Cache Invalidation

When the Server indicates that cached information is no longer valid, the Client must update or remove the affected local cached state.

Examples include:

- Project membership removal
- Project deletion
- Message deletion
- Role changes
- Profile privacy changes
- Resource access changes

Cached data must never be used to bypass current Server authorization.

### Authentication Failure During Reconnection

If the stored Server session is expired, revoked, or otherwise invalid, the Client must stop attempting authenticated synchronization and require the user to authenticate with that Server again.

Locally cached information may remain on the device according to Client cache policy, but it must not be presented as proof of current Server access.

---

## Internal Application Architecture

KMJG Hub separates application responsibilities into clear internal boundaries.

The architecture should remain simple enough for v1 while avoiding unnecessary coupling between unrelated features.

### Server Internal Architecture

The Go Server should separate transport, application logic, persistence, and external integrations.

Conceptually:

    HTTP / WebSocket
           |
           v
       Handlers
           |
           v
    Application Services
           |
      +----+-------------------+
      |                        |
      v                        v
  Repositories           Integrations
      |                        |
      v                        v
  PostgreSQL             Git Providers
                               |
                               v
                            GitHub

Application services contain the primary business rules for features such as:

- Authentication
- Users
- Friends and blocking
- Projects
- Membership and roles
- Invitations
- Chat and Direct Messages
- Tasks
- Notifications
- Presence
- Git integration
- File management

HTTP handlers and WebSocket handlers should translate network requests and events into application operations rather than contain the core business rules themselves.

Database-specific logic should remain separated from HTTP and WebSocket transport logic.

GitHub-specific behavior should remain behind the Git provider integration boundary.

File storage operations should remain behind the storage abstraction.

The exact Go package structure will be selected during implementation and may evolve as the codebase grows.

### Desktop Client Internal Architecture

The Desktop Client separates user interface state, Server communication, local persistence, and privileged native operations.

Conceptually:

    React UI
       |
       +-------------------+
       |                   |
       v                   v
    Client State       API / Realtime
                           |
                           v
                       Go Server

       |
       v
    Tauri Interface
       |
       v
    Rust Native Layer
       |
       +----> Local Git
       +----> Filesystem
       +----> External Tools
       +----> Secure Credential Storage
       |
       v
    SQLite Cache

React components should not contain unnecessary direct knowledge of operating-system-specific behavior.

Native operations are exposed through explicitly defined Tauri interfaces.

Server communication should be isolated from presentation components where practical so network behavior can be changed without rewriting unrelated UI.

### Shared Data Contracts

Communication between Client and Server must use explicitly defined request, response, and event structures.

The Client must not depend directly on Go internal data structures or PostgreSQL database schemas.

The network protocol forms the compatibility boundary between Client and Server.

Changes to internal Server implementation should not require Client changes unless the public application protocol changes.

## User Profile and Privacy Architecture

User profiles are Server-managed resources.

Profile data may include:

- Display Name
- Username
- Avatar
- Bio
- Presence
- Work Status
- Current Project
- Current Task
- Current Branch
- Connected Git provider information
- Repositories the user chooses to expose

### Field-Level Privacy

Supported profile fields may have independent privacy settings.

KMJG Hub v1 supports the following privacy audiences where applicable:

- Everyone on the same Server
- Friends
- Project Members
- Friends and Project Members
- Nobody

The Server is authoritative for profile privacy enforcement.

When profile information is requested, the Server must evaluate the relationship between the profile owner and the requesting user before returning protected fields.

Relevant relationships may include:

- Same Server
- Friendship
- Shared Project membership

The Client must not receive protected profile data and merely hide it in the user interface.

A modified or third-party Client must not be able to bypass profile privacy by directly requesting fields that the viewer is not authorized to access.

### Live Work Information

Presence, Work Status, Current Project, Current Task, and Current Branch may represent live or contextual user activity.

When a user is Offline, KMJG Hub must not present Work Status, Current Task, or Current Branch as current live information.

Static profile information may remain available according to the owner's configured privacy settings.

### Repository Profile Visibility

Users may choose which repositories, if any, are exposed through their profile.

Repository profile visibility is separate from Git provider repository authorization.

Displaying a repository on a KMJG Hub profile must not:

- Make the repository public
- Grant repository access
- Expose repository files
- Automatically expose branches or commits
- Override Git provider permissions

If the Git provider identifies a repository as Private, the Client must warn the user before enabling profile visibility for that repository.

Only repository information explicitly permitted by the user and allowed by the applicable profile privacy rules may be returned to a viewer.

### Privacy Changes

Privacy-setting changes are persistent Server-managed state.

After a privacy setting changes, subsequent API responses and real-time updates must respect the new authorization state.

Locally cached profile information that is no longer permitted must not continue to be presented as accessible information.

The Client should invalidate or update affected cached profile data when the relevant privacy or relationship state changes.

---

## Security Architecture

KMJG Hub follows a Server-authoritative security model.

The Server does not trust Client-provided authorization decisions.

### Trust Boundaries

Important trust boundaries include:

- Desktop Client to KMJG Hub Server
- React frontend to Tauri native layer
- KMJG Hub Server to PostgreSQL
- KMJG Hub Server to persistent file storage
- KMJG Hub Server to external Git providers
- KMJG Hub deployment to public or organizational networks

Data crossing a trust boundary must be validated according to its context.

### Input Validation

The Server validates untrusted Client input before using it.

Validation applies to areas such as:

- Resource identifiers
- User input
- File metadata
- Upload sizes
- Project membership operations
- Role changes
- Invitation parameters
- Git provider operations
- Network event payloads

The Server must not assume that requests originate from an official or unmodified KMJG Hub Client.

### Database Security

Application database queries must use safe parameterized query mechanisms.

User-controlled values must not be directly concatenated into SQL statements.

Database credentials are deployment secrets and must not be exposed to Desktop Clients.

### File Security

Client-provided file names must not determine arbitrary Server filesystem paths.

The Server controls storage identifiers and storage locations.

File upload and download operations require authorization before protected content is accessed.

The storage layer must prevent path traversal and access outside configured KMJG Hub storage.

### Native Client Security

The React frontend must not receive unrestricted operating-system access.

Privileged operations exposed through Tauri should be limited to the capabilities required by KMJG Hub.

Arguments passed from the frontend to native commands must be validated before privileged operations are performed.

External tool launching must not construct unsafe shell commands from untrusted Server-controlled text.

### Transport Security

Communication over untrusted networks uses HTTPS and secure WebSocket transport.

Plain HTTP may be used only in explicitly trusted development or local deployment scenarios where appropriate.

Production deployments exposed beyond a trusted local environment must use encrypted transport.

### Secrets

Secrets must not be hardcoded into the KMJG Hub source code or committed to the repository.

Secrets may include:

- Database credentials
- Git provider credentials
- OAuth secrets
- Session-related secrets where required
- Administrative credentials
- External service credentials introduced in the future

Server secrets are provided through deployment configuration or another appropriate secret-management mechanism.

Desktop Clients must never receive Server-only secrets.

---

## Server Configuration

KMJG Hub Server uses deployment-provided configuration rather than machine-specific values embedded in application code.

Configuration may include:

- Server listening address and port
- Public Server URL where required
- PostgreSQL connection information
- Persistent storage location
- Upload limits
- Project storage limits
- Backup configuration
- Authentication configuration
- Git provider configuration
- Retention-related configuration where permitted
- Logging configuration

Environment variables and configuration files may be used where appropriate.

Sensitive configuration must be handled separately from non-sensitive configuration where practical.

The exact configuration format will be selected during implementation.

### Deployment Portability

Configuration must allow the same KMJG Hub Server application to run on:

- A developer workstation
- An office Linux machine
- A dedicated Server
- A virtual machine
- A VPS
- A containerized environment

Changing deployment infrastructure should primarily require configuration and data migration rather than application source-code changes.

---

## Logging and Observability

KMJG Hub Server must provide operational logging suitable for self-hosted administration and troubleshooting.

Logs may include information such as:

- Server startup and shutdown
- Database connectivity
- Client connection and disconnection events
- Authentication success or failure
- Administrative operations
- Application errors
- Git provider integration failures
- File transfer failures
- Backup operations
- Cleanup operations

### Sensitive Logging

Logs must not contain sensitive secrets.

The Server must avoid logging:

- Plain-text passwords
- Session tokens
- OAuth access tokens
- OAuth client secrets
- Database passwords
- Private message contents unless explicitly required by a future controlled diagnostic mechanism
- Raw private file contents

Sensitive values should be omitted or safely redacted.

### Structured Logging

The Go Server should use structured logging where practical so logs can be read by humans and processed by operational tools.

Log entries should contain useful context such as:

- Timestamp
- Severity
- Component
- Operation
- Relevant non-sensitive identifiers
- Error information

Logging must not become an authorization or data-storage mechanism.

PostgreSQL remains the authoritative store for application data that requires persistence.

---

## Protocol Versioning and Compatibility

The KMJG Hub Desktop Client and Server are independently deployable components and may not always be updated at exactly the same time.

The Client-Server protocol must therefore provide an explicit compatibility boundary.

### API Versioning

HTTP API routes use an explicit major version.

Example:

    /api/v1/...

Breaking protocol changes should introduce a new major API version rather than silently changing the meaning of an existing contract.

Compatible additions may be introduced within the existing API version where they do not break supported Clients.

### Real-Time Protocol Versioning

WebSocket communication must also have an identifiable protocol version or equivalent compatibility mechanism.

The Client and Server should be able to determine whether they can communicate using compatible real-time event contracts.

Real-time event payloads should use explicitly defined event types and data structures.

Conceptually:

    {
      "type": "project.message.created",
      "data": { ... }
    }

The exact event envelope will be defined during API implementation.

### Client and Server Compatibility

The Server should expose sufficient version information for the Client to determine whether the Server is compatible.

If the Client and Server are incompatible, the Client must display a clear error rather than failing unpredictably.

The architecture should allow reasonable compatibility between nearby Client and Server releases where practical.

The exact supported compatibility policy will be defined when KMJG Hub begins producing versioned releases.

### Database Migrations

Changes to the PostgreSQL schema must be managed through explicit database migrations.

A Server update must not depend on manually editing production database tables.

Database migrations should be:

- Version controlled
- Applied in a defined order
- Repeatable across deployments
- Included as part of the Server deployment and upgrade process

Migration tooling will be selected during implementation.

Changes to the Client SQLite schema must also use a controlled migration strategy where persistent local data needs to survive Client updates.

---

## v1 System Architecture

The high-level KMJG Hub v1 architecture is:

    +------------------------------------------------------+
    |                KMJG Hub Desktop Client               |
    |                                                      |
    |  React + TypeScript + Vite                           |
    |            |                                         |
    |            +-------- API / WebSocket Client          |
    |            |                                         |
    |            +-------- Tauri Interface                 |
    |                         |                            |
    |                         v                            |
    |                    Rust Native Layer                 |
    |                    |       |       |                 |
    |                    |       |       +--> External     |
    |                    |       |            Dev Tools    |
    |                    |       |                         |
    |                    |       +----------> Local Git    |
    |                    |                                 |
    |                    +------------------> Filesystem    |
    |                                                      |
    |                    SQLite Cache                      |
    +--------------------------+---------------------------+
                               |
                         HTTPS / WSS
                               |
                               v
    +------------------------------------------------------+
    |                   KMJG Hub Server                    |
    |                                                      |
    |                 Go Modular Monolith                  |
    |                                                      |
    |   HTTP API --------+                                 |
    |                    |                                 |
    |   WebSocket -------+--> Application Services         |
    |                            |                         |
    |              +-------------+-------------+           |
    |              |             |             |           |
    |              v             v             v           |
    |         PostgreSQL     File Storage   Git Provider   |
    |                                        Integration   |
    |                                             |        |
    |                                             v        |
    |                                           GitHub     |
    +------------------------------------------------------+

The Server is the authoritative source for shared KMJG Hub application state.

The Desktop Client maintains local state and cached Server data while delegating privileged operating-system operations to the Tauri native layer.

---

## v1 Deployment Model

A typical small-team self-hosted deployment may use:

    Internet / Organization Network
                 |
                 v
        hub.example.com
                 |
                 v
      Secure Network Exposure
       /                  \
      /                    \
Cloudflare Tunnel      Reverse Proxy
    (optional)            (optional)
      \                    /
       \                  /
                 |
                 v
    +-----------------------------+
    |        Linux Host           |
    |                             |
    |   Docker Compose            |
    |                             |
    |   +---------------------+   |
    |   | KMJG Hub Server     |   |
    |   | Go                  |   |
    |   +----------+----------+   |
    |              |              |
    |   +----------v----------+   |
    |   | PostgreSQL          |   |
    |   +---------------------+   |
    |                             |
    |   Persistent Storage        |
    |   +-- database-data         |
    |   +-- file-storage          |
    |   +-- backups               |
    +-----------------------------+

Cloudflare Tunnel, reverse proxies, domains, and TLS termination belong to deployment infrastructure rather than the core KMJG Hub application.

A deployment may also operate entirely inside a trusted local network.

---

## Architecture Boundaries for v1

KMJG Hub v1 intentionally uses a relatively simple architecture appropriate for small self-hosted software teams.

The architecture does not require:

- Microservices
- Kubernetes
- Distributed databases
- Message brokers
- Event streaming platforms
- Peer-to-peer file transfer
- End-to-end encrypted messaging
- Built-in voice or video infrastructure
- Remote desktop infrastructure
- Built-in AI model infrastructure
- A centralized KMJG Hub cloud service

These technologies may be evaluated in the future only when supported by concrete product requirements.

### Future Extensibility

The v1 architecture preserves selected boundaries that allow future capabilities to be introduced without requiring them now.

Examples include:

- Additional Git providers behind the Git provider integration boundary
- Alternative persistent file storage backends
- Peer-to-peer Direct File Transfer transport
- macOS Desktop Client support
- Additional Client types using the defined Server protocol
- More advanced synchronization strategies
- Additional deployment environments

Future extensibility must not justify unnecessary complexity in the v1 implementation.

---

## Architecture Principles

KMJG Hub v1 follows these architectural principles:

1. The Server is authoritative for shared application data.
2. Clients are never trusted to enforce Server permissions.
3. Project roles and Server administration are separate authority domains.
4. KMJG Project access and Git provider repository access are independent.
5. Persistent data and real-time delivery are separate concerns.
6. PostgreSQL stores authoritative relational Server data.
7. SQLite stores derived local Client data and cache.
8. Persistent file content is handled through a storage abstraction.
9. Privileged desktop operations are isolated behind the Tauri native layer.
10. Deployment infrastructure is separate from application architecture.
11. Self-hosting must not depend on a centralized KMJG Hub service.
12. Security-sensitive secrets must not be embedded in source code or exposed to Clients.
13. Offline Client state must not override authoritative Server state.
14. Architecture should remain simple until product requirements justify additional complexity.
