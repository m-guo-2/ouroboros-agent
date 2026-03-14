## MODIFIED Requirements

### Requirement: Object upload contract
The system SHALL support object upload through a stable contract that accepts object content, target bucket context, object key or key-generation inputs, and optional content metadata such as content type.

#### Scenario: Upload object with explicit key
- **WHEN** a caller uploads content and provides an explicit object key
- **THEN** the capability stores the object under that key and returns the stored object's canonical location metadata

#### Scenario: Upload object with generated key
- **WHEN** a caller uploads content without providing an explicit object key
- **THEN** the capability generates a valid object key according to the configured naming strategy and optional prefix before storing the object

#### Scenario: Upload PNG image from card rendering
- **WHEN** the card rendering capability uploads a PNG image with content type `image/png`
- **THEN** the capability stores it and returns metadata including the object key, which can be used to generate a presigned URL

### Requirement: Presigned URL generation
The system SHALL support generating presigned GET URLs for stored objects, allowing temporary public access without authentication.

#### Scenario: Generate presigned URL with custom expiry
- **WHEN** a caller requests a presigned URL for an existing object key with a specified expiry duration
- **THEN** the capability returns a valid presigned URL that grants read access for the specified duration

#### Scenario: Generate presigned URL for rendered card image
- **WHEN** the card rendering capability requests a presigned URL for an uploaded PNG with a 7-day expiry
- **THEN** the capability returns a URL accessible for at least 7 days without authentication
