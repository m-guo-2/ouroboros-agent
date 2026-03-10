## ADDED Requirements

### Requirement: Shared object storage abstraction
The system SHALL provide a shared object storage capability that exposes stable upload and download operations for MinIO/S3-compatible object storage without requiring business modules to call the vendor SDK directly.

#### Scenario: Business module uses shared OSS interface
- **WHEN** a business module needs to upload or download an object
- **THEN** it can do so through the shared OSS capability's public interface without depending on MinIO-specific client types

### Requirement: Configurable MinIO-compatible connection
The system SHALL allow the shared OSS capability to connect to a MinIO/S3-compatible service through configuration, including endpoint, bucket, credentials, transport security settings, and optional object key prefix.

#### Scenario: Initialize client from configured endpoint
- **WHEN** the shared OSS capability starts with endpoint configuration such as `115.190.14.209:2012`
- **THEN** it initializes the underlying client from configuration rather than from hard-coded values

#### Scenario: Reject incomplete storage configuration
- **WHEN** required storage configuration such as endpoint, bucket, or credentials is missing or invalid
- **THEN** the capability returns a clear configuration error before attempting upload or download

### Requirement: Object upload contract
The system SHALL support object upload through a stable contract that accepts object content, target bucket context, object key or key-generation inputs, and optional content metadata such as content type.

#### Scenario: Upload object with explicit key
- **WHEN** a caller uploads content and provides an explicit object key
- **THEN** the capability stores the object under that key and returns the stored object's canonical location metadata

#### Scenario: Upload object with generated key
- **WHEN** a caller uploads content without providing an explicit object key
- **THEN** the capability generates a valid object key according to the configured naming strategy and optional prefix before storing the object

### Requirement: Object download contract
The system SHALL support object download through a stable contract that retrieves object content and metadata by bucket context and object key.

#### Scenario: Download existing object
- **WHEN** a caller requests an object by a valid object key that exists in storage
- **THEN** the capability returns a readable content stream or bytes handle together with the object's available metadata

#### Scenario: Report missing object consistently
- **WHEN** a caller requests an object key that does not exist
- **THEN** the capability returns a stable not-found error classification instead of exposing only raw vendor error text

### Requirement: Stable error classification
The system SHALL normalize storage failures into stable error categories so that callers can distinguish configuration, authentication, not-found, transport, and internal I/O failures.

#### Scenario: Surface authentication failure
- **WHEN** upload or download fails because the configured credentials are rejected by the storage service
- **THEN** the capability returns an authentication-related error classification while retaining the underlying error for diagnostics

#### Scenario: Surface transport timeout failure
- **WHEN** upload or download fails because the storage service is unreachable or times out
- **THEN** the capability returns a transport-related error classification that callers can handle consistently

### Requirement: Testable storage interface
The system SHALL define the shared OSS capability behind an interface that can be replaced by mock or fake implementations in unit tests.

#### Scenario: Replace storage implementation in tests
- **WHEN** a unit test exercises code that depends on the shared OSS capability
- **THEN** the test can inject a fake or mock implementation without requiring a live MinIO service
