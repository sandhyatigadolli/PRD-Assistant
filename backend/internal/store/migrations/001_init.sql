CREATE TABLE IF NOT EXISTS sessions (
  id           CHAR(24)     NOT NULL PRIMARY KEY,
  title        VARCHAR(255) NOT NULL,
  prd_type     VARCHAR(32)  NOT NULL,
  repeat_state JSON         NOT NULL,
  generated    MEDIUMTEXT   NULL,
  generated_at DATETIME     NULL,
  created_at   DATETIME     NOT NULL,
  updated_at   DATETIME     NOT NULL
) CHARACTER SET utf8mb4;

CREATE TABLE IF NOT EXISTS answers (
  id         CHAR(24)    NOT NULL PRIMARY KEY,
  session_id CHAR(24)    NOT NULL,
  section    VARCHAR(64) NOT NULL,
  item       INT         NOT NULL DEFAULT 0,
  field      VARCHAR(64) NOT NULL,
  content    MEDIUMTEXT  NOT NULL,
  tags       JSON        NOT NULL,
  skipped    TINYINT(1)  NOT NULL DEFAULT 0,
  status     VARCHAR(16) NOT NULL DEFAULT 'accepted',
  feedback   JSON        NOT NULL,
  suggestion MEDIUMTEXT  NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  UNIQUE KEY uq_answer (session_id, section, item, field),
  KEY idx_answers_session (session_id),
  CONSTRAINT fk_answers_session FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
) CHARACTER SET utf8mb4;

CREATE TABLE IF NOT EXISTS answer_versions (
  id         BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
  answer_id  CHAR(24)    NOT NULL,
  content    MEDIUMTEXT  NOT NULL,
  tags       JSON        NOT NULL,
  created_at DATETIME(6) NOT NULL,
  KEY idx_versions_answer (answer_id),
  CONSTRAINT fk_versions_answer FOREIGN KEY (answer_id) REFERENCES answers(id) ON DELETE CASCADE
) CHARACTER SET utf8mb4;

CREATE TABLE IF NOT EXISTS messages (
  id         BIGINT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
  session_id CHAR(24)    NOT NULL,
  role       VARCHAR(16) NOT NULL,
  content    MEDIUMTEXT  NOT NULL,
  created_at DATETIME(6) NOT NULL,
  KEY idx_messages_session (session_id),
  CONSTRAINT fk_messages_session FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
) CHARACTER SET utf8mb4;
