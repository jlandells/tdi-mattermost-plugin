import {getConfig} from 'mattermost-redux/selectors/entities/general';
import {Client4} from 'mattermost-redux/client';

const pluginId = 'com.archtis.mattermost-policy-plugin';

function getPluginRoute(state) {
  const config = getConfig(state);
  let basePath = '';
  if (config?.SiteURL) {
    basePath = new URL(config.SiteURL).pathname;
    if (basePath && basePath.endsWith('/')) {
      basePath = basePath.slice(0, -1);
    }
  }
  return basePath + '/plugins/' + pluginId;
}

function getCSRFToken() {
  const match = document.cookie.match(/MMCSRF=([^;]+)/);
  return match ? decodeURIComponent(match[1]) : '';
}

function getFetchOpts(method, body) {
  const opts = (Client4.getOptions && Client4.getOptions({method})) || {method};
  opts.credentials = opts.credentials || 'same-origin';
  opts.headers = opts.headers || {};
  opts.headers['X-CSRF-Token'] = getCSRFToken();
  if (body) {
    opts.body = typeof body === 'string' ? body : JSON.stringify(body);
    opts.headers['Content-Type'] = 'application/json';
  }
  return opts;
}

export async function fetchPolicies(channelId) {
  const store = window.mattermostPluginStore;
  if (!store) throw new Error('Store not available');
  const state = store.getState();
  const baseUrl = getPluginRoute(state);
  const url = channelId ? `${baseUrl}/api/policies?channel_id=${encodeURIComponent(channelId)}` : `${baseUrl}/api/policies`;
  const res = await fetch(url, getFetchOpts('GET'));
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || 'Failed to fetch policies');
  return {policies: data.policies || [], currentPolicy: data.current_policy || null};
}

export async function classifyChannel(channelId, policyId) {
  const store = window.mattermostPluginStore;
  if (!store) throw new Error('Store not available');
  const state = store.getState();
  const baseUrl = getPluginRoute(state);
  const res = await fetch(baseUrl + '/api/classify', getFetchOpts('POST', {channel_id: channelId, policy_id: policyId}));
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || 'Failed to classify channel');
  return data;
}
