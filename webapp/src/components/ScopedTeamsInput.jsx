import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {Client4} from 'mattermost-redux/client';

import './ScopedTeamsInput.css';

const TEAMS_PAGE_SIZE = 200;
const MAX_SUGGESTIONS = 8;

function toArray(value) {
  if (Array.isArray(value)) return value;
  if (value == null || value === '') return [];
  return [];
}

export default function ScopedTeamsInput(props) {
  const {id, label, helpText, value, disabled, onChange, setSaveNeeded} = props;

  const [selected, setSelected] = useState(() => toArray(value));
  const [teams, setTeams] = useState([]);
  const [teamsError, setTeamsError] = useState(null);
  const [inputValue, setInputValue] = useState('');
  const [focused, setFocused] = useState(false);
  const inputRef = useRef(null);

  // Sync external value changes (e.g. settings reset) into local state.
  useEffect(() => {
    setSelected(toArray(value));
  }, [value]);

  // Load the team list once. We paginate until the server returns less than a
  // full page, so very large instances still see every team.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      const collected = [];
      try {
        for (let page = 0; page < 50; page++) {
          // eslint-disable-next-line no-await-in-loop
          const batch = await Client4.getTeams(page, TEAMS_PAGE_SIZE);
          if (cancelled) return;
          if (!batch || batch.length === 0) break;
          collected.push(...batch);
          if (batch.length < TEAMS_PAGE_SIZE) break;
        }
        if (!cancelled) {
          setTeams(collected);
        }
      } catch (err) {
        if (!cancelled) {
          setTeamsError(err?.message || 'Failed to load teams');
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const selectedSet = useMemo(() => new Set(selected), [selected]);

  const suggestions = useMemo(() => {
    const needle = inputValue.trim().toLowerCase();
    return teams
      .filter((t) => !selectedSet.has(t.name))
      .filter((t) => {
        if (!needle) return true;
        return (
          t.name.toLowerCase().includes(needle) ||
          (t.display_name || '').toLowerCase().includes(needle)
        );
      })
      .slice(0, MAX_SUGGESTIONS);
  }, [teams, selectedSet, inputValue]);

  const commit = useCallback(
    (next) => {
      setSelected(next);
      onChange(id, next);
      if (setSaveNeeded) setSaveNeeded();
    },
    [id, onChange, setSaveNeeded]
  );

  const addTeam = useCallback(
    (teamName) => {
      const name = (teamName || '').trim().toLowerCase();
      if (!name || selectedSet.has(name)) return;
      commit([...selected, name]);
      setInputValue('');
      if (inputRef.current) inputRef.current.focus();
    },
    [commit, selected, selectedSet]
  );

  const removeTeam = useCallback(
    (teamName) => {
      commit(selected.filter((n) => n !== teamName));
    },
    [commit, selected]
  );

  const onKeyDown = (e) => {
    if (e.key === 'Backspace' && inputValue === '' && selected.length > 0) {
      e.preventDefault();
      commit(selected.slice(0, -1));
      return;
    }
    if (e.key === 'Enter' || e.key === ',' || e.key === 'Tab') {
      // Enter / comma / Tab → commit the top suggestion (if any) or the raw
      // typed text (validation happens server-side; typos are logged).
      const candidate = suggestions[0]?.name || inputValue.trim();
      if (candidate) {
        e.preventDefault();
        addTeam(candidate);
      }
    }
    if (e.key === 'Escape') {
      setInputValue('');
    }
  };

  return (
    <div className="form-group scoped-teams-input" data-setting-id={id}>
      <label className="control-label col-sm-4" htmlFor={id}>
        {label}
      </label>
      <div className="col-sm-8">
        <div
          className={`scoped-teams-chips ${focused ? 'is-focused' : ''} ${disabled ? 'is-disabled' : ''}`}
          onClick={() => inputRef.current && inputRef.current.focus()}
        >
          {selected.map((name) => (
            <span key={name} className="scoped-teams-chip">
              {name}
              {!disabled && (
                <button
                  type="button"
                  className="scoped-teams-chip-remove"
                  aria-label={`Remove ${name}`}
                  onClick={(e) => {
                    e.stopPropagation();
                    removeTeam(name);
                  }}
                >
                  ×
                </button>
              )}
            </span>
          ))}
          <input
            id={id}
            ref={inputRef}
            type="text"
            className="scoped-teams-input-field"
            placeholder={selected.length === 0 ? 'All teams (no restriction)' : 'Add a team…'}
            value={inputValue}
            disabled={disabled}
            onChange={(e) => setInputValue(e.target.value)}
            onKeyDown={onKeyDown}
            onFocus={() => setFocused(true)}
            onBlur={() => setTimeout(() => setFocused(false), 120)}
            autoComplete="off"
          />
        </div>
        {focused && suggestions.length > 0 && (
          <ul className="scoped-teams-suggestions">
            {suggestions.map((t) => (
              <li
                key={t.id}
                className="scoped-teams-suggestion"
                // onMouseDown fires before onBlur, so we don't lose focus mid-click.
                onMouseDown={(e) => {
                  e.preventDefault();
                  addTeam(t.name);
                }}
              >
                <span className="scoped-teams-suggestion-name">{t.name}</span>
                {t.display_name && t.display_name !== t.name && (
                  <span className="scoped-teams-suggestion-display"> — {t.display_name}</span>
                )}
              </li>
            ))}
          </ul>
        )}
        {helpText && <div className="help-text">{helpText}</div>}
        {teamsError && (
          <div className="help-text scoped-teams-error">
            Could not load team list: {teamsError}. You can still type team names manually.
          </div>
        )}
      </div>
    </div>
  );
}
