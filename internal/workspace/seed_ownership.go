package workspace

// A saved fingerprint may restore its own historical seed. No other writer
// may consume a seed durably reserved by a prepared batch environment.
func seedAvailable(query profileQuery, seed, profileID, environmentID string) (bool, error) {
	var occupied bool
	err := query.QueryRow(`SELECT EXISTS(SELECT 1 FROM fingerprints WHERE seed=? AND id<>?) OR EXISTS(SELECT 1 FROM fingerprint_revisions WHERE json_extract(profile_json,'$.seed')=? AND fingerprint_id<>?) OR EXISTS(SELECT 1 FROM batch_items WHERE identity_json IS NOT NULL AND json_extract(identity_json,'$.profile.seed')=? AND json_extract(identity_json,'$.environmentId')<>?)`, seed, profileID, seed, profileID, seed, environmentID).Scan(&occupied)
	return !occupied, err
}
