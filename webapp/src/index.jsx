import React from 'react';
import {Provider} from 'react-redux';
import manifest from '../../plugin.json';
import ClassifyChannelRHS from './components/ClassifyChannelRHS';

const pluginId = manifest.id;

// Shield with checkmark icon for "Classify channel"
const ClassifyIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" style={{verticalAlign: 'middle'}}>
    <path d="M12 1L3 5v6c0 5.55 3.84 10.74 9 12 5.16-1.26 9-6.45 9-12V5l-9-4zm-2 16l-4-4 1.41-1.41L10 14.17l6.59-6.59L18 9l-8 8z"/>
  </svg>
);

const RHSComponent = () => (
  <Provider store={window.mattermostPluginStore}>
    <ClassifyChannelRHS />
  </Provider>
);

class Plugin {
  initialize(registry, store) {
    window.mattermostPluginStore = store;
    this.rhsPlugin = registry.registerRightHandSidebarComponent(RHSComponent, 'Classify Channel');
    registry.registerChannelHeaderButtonAction(
      <ClassifyIcon />,
      () => store.dispatch(this.rhsPlugin.toggleRHSPlugin),
      'Classify Channel'
    );
  }
}

window.registerPlugin(pluginId, new Plugin());
