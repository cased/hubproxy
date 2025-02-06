CREATE TABLE events (
    id VARCHAR(36) PRIMARY KEY,
    type VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL,
    status VARCHAR(20) NOT NULL,
    error TEXT,
    repository VARCHAR(255),
    sender VARCHAR(255)
);

CREATE INDEX idx_created_at ON events (created_at);
CREATE INDEX idx_type ON events (type);
CREATE INDEX idx_status ON events (status);
CREATE INDEX idx_repository ON events (repository);
CREATE INDEX idx_sender ON events (sender);
