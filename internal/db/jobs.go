package db

// Job-row persistence (scan/refresh/firmware jobs). MOVED FROM db.go —
// db-layer split by domain (post-v0.5.2 review item 6); bodies unchanged.

import "shellyadmin/internal/models"

func (db *DB) MarkRunningJobsInterrupted() error {
	_, err := db.sql.Exec(`UPDATE jobs
		SET status = 'interrupted', error = 'service restarted', updated_at = ?
		WHERE status = 'running'`, now())
	return err
}

func (db *DB) CreateJob(jobType, restartPolicy, payload string, total int) (int64, error) {
	res, err := db.sql.Exec(`INSERT INTO jobs(type, status, restart_policy, done, total, payload, result, error, created_at, updated_at)
		VALUES (?, 'running', ?, 0, ?, ?, '', '', ?, ?)`, jobType, restartPolicy, total, payload, now(), now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) UpdateJobProgress(id int64, done, total int, result string) error {
	_, err := db.sql.Exec(`UPDATE jobs
		SET done = ?, total = ?, result = ?, updated_at = ?
		WHERE id = ?`, done, total, result, now(), id)
	return err
}

func (db *DB) IncrementJobDone(id int64) error {
	_, err := db.sql.Exec(`UPDATE jobs
		SET done = done + 1, updated_at = ?
		WHERE id = ? AND status = 'running'`, now(), id)
	return err
}

func (db *DB) CompleteJob(id int64, status, result, errText string, done, total int) error {
	_, err := db.sql.Exec(`UPDATE jobs
		SET status = ?, result = ?, error = ?, done = ?, total = ?, updated_at = ?
		WHERE id = ?`, status, result, errText, done, total, now(), id)
	return err
}

func (db *DB) InterruptJob(id int64, errText string) error {
	_, err := db.sql.Exec(`UPDATE jobs
		SET status = 'interrupted', error = ?, updated_at = ?
		WHERE id = ? AND status = 'running'`, errText, now(), id)
	return err
}

func (db *DB) GetLatestJob(jobType string) (models.Job, error) {
	row := db.sql.QueryRow(`SELECT id, type, status, restart_policy, done, total, payload, result, error, created_at, updated_at
		FROM jobs WHERE type = ? ORDER BY id DESC LIMIT 1`, jobType)
	return scanJob(row)
}

func (db *DB) GetJob(id int64) (models.Job, error) {
	row := db.sql.QueryRow(`SELECT id, type, status, restart_policy, done, total, payload, result, error, created_at, updated_at
		FROM jobs WHERE id = ?`, id)
	return scanJob(row)
}

func (db *DB) ListInterruptedRestartableJobs() ([]models.Job, error) {
	rows, err := db.sql.Query(`SELECT id, type, status, restart_policy, done, total, payload, result, error, created_at, updated_at
		FROM jobs
		WHERE status = 'interrupted' AND restart_policy = 'auto'
		ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func scanJob(scanner interface{ Scan(dest ...any) error }) (models.Job, error) {
	var job models.Job
	err := scanner.Scan(&job.ID, &job.Type, &job.Status, &job.RestartPolicy, &job.Done, &job.Total, &job.Payload, &job.Result, &job.Error, &job.CreatedAt, &job.UpdatedAt)
	return job, err
}
