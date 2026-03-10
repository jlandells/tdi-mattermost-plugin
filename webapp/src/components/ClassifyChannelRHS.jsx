import React, {useState, useEffect} from 'react';
import {useSelector} from 'react-redux';
import {getCurrentChannel} from 'mattermost-redux/selectors/entities/channels';
import {canManageChannelMembers} from 'mattermost-redux/selectors/entities/channels';
import {fetchPolicies, classifyChannel} from '../api';
import './ClassifyChannelRHS.css';

export default function ClassifyChannelRHS() {
  const channel = useSelector(getCurrentChannel);
  const isChannelAdmin = useSelector(canManageChannelMembers);
  const [policies, setPolicies] = useState([]);
  const [currentPolicy, setCurrentPolicy] = useState(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState(null);
  const [selectedPolicyId, setSelectedPolicyId] = useState('');

  useEffect(() => {
    setError(null);
    setSuccess(null);
    setSelectedPolicyId('');
    setCurrentPolicy(null);
    if (!channel?.id || !isChannelAdmin) {
      setLoading(false);
      setPolicies([]);
      return;
    }
    setLoading(true);
    fetchPolicies(channel.id)
      .then(({policies: list, currentPolicy: curr}) => {
        setPolicies(list || []);
        setCurrentPolicy(curr || null);
        if (list?.length) setSelectedPolicyId(list[0].id);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [channel?.id, isChannelAdmin]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!channel?.id || !selectedPolicyId) return;
    setSubmitting(true);
    setError(null);
    setSuccess(null);
    try {
      const result = await classifyChannel(channel.id, selectedPolicyId);
      const policyName = policies.find((p) => p.id === selectedPolicyId)?.name || 'Policy';
      setCurrentPolicy({id: selectedPolicyId, name: policyName});
      setSuccess(`Channel "${result.channel_name}" has been classified. A confirmation was posted in the channel.`);
      setSelectedPolicyId('');
    } catch (err) {
      setError(err.message);
    } finally {
      setSubmitting(false);
    }
  };

  if (!channel?.id) {
    return (
      <div className="classify-rhs">
        <div className="classify-rhs-header">
          <h3>Classify Channel</h3>
        </div>
        <div className="classify-rhs-body classify-rhs-empty">
          <p>Select a channel to classify it with an access control policy.</p>
        </div>
      </div>
    );
  }

  if (!isChannelAdmin) {
    return (
      <div className="classify-rhs">
        <div className="classify-rhs-header">
          <h3>Classify Channel</h3>
          <span className="classify-rhs-channel">{channel.display_name || channel.name}</span>
        </div>
        <div className="classify-rhs-body classify-rhs-empty">
          <p className="classify-rhs-message classify-rhs-error">
            Only channel admins can classify this channel.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="classify-rhs">
      <div className="classify-rhs-header">
        <h3>Classify Channel</h3>
        <span className="classify-rhs-channel">{channel.display_name || channel.name}</span>
      </div>
      <div className="classify-rhs-body">
        {!loading && (
          <p className="classify-rhs-current">
            {currentPolicy ? (
              <>Current policy: <strong>{currentPolicy.name}</strong></>
            ) : (
              <>No policy assigned</>
            )}
          </p>
        )}
        <p className="classify-rhs-intro">
          Apply an access control policy to control who can join this channel. Until classified, user access may be restricted.
        </p>

        {loading ? (
          <div className="classify-rhs-loading">
            <div className="classify-rhs-spinner" />
            <span>Loading policies…</span>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="classify-rhs-form">
            <label htmlFor="classify-policy">Policy</label>
            <select
              id="classify-policy"
              value={selectedPolicyId}
              onChange={(e) => setSelectedPolicyId(e.target.value)}
              disabled={submitting || !policies.length}
            >
              {!policies.length && <option value="">No policies available</option>}
              {policies.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name} {p.active ? '(active)' : '(inactive)'}
                </option>
              ))}
            </select>

            {error && (
              <div className="classify-rhs-message classify-rhs-error">{error}</div>
            )}
            {success && (
              <div className="classify-rhs-message classify-rhs-success">{success}</div>
            )}

            <button
              type="submit"
              className="classify-rhs-submit"
              disabled={submitting || !selectedPolicyId || !policies.length}
            >
              {submitting ? 'Applying…' : 'Apply Policy'}
            </button>
          </form>
        )}
      </div>
    </div>
  );
}
