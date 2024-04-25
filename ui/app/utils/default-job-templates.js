/**
 * Copyright (c) HashiCorp, Inc.
 * SPDX-License-Identifier: BUSL-1.1
 */

import helloWorld from './default_jobs/hello-world';
import parameterized from './default_jobs/parameterized';
import serviceDiscovery from './default_jobs/service-discovery';
import variables from './default_jobs/variables';
import actions from './default_jobs/actions';

export default [
  {
    id: 'nomad/job-templates/default/hello-world',
    keyValues: [
      {
        key: 'template',
        value: helloWorld,
      },
      {
        key: 'description',
        value: 'A simple job that runs a single task on a single node',
      },
    ],
  },
  {
    id: 'nomad/job-templates/default/actions',
    keyValues: [
      {
        key: 'template',
        value: actions,
      },
      {
        key: 'description',
        value:
          'Nomad Actions let job authors describe commands that can be run in one click from the UI or one command from the CLI. This example job shows how to use them to simulate development on a Redis instance.',
      },
    ],
  },
  {
    id: 'nomad/job-templates/default/parameterized-job',
    keyValues: [
      {
        key: 'template',
        value: parameterized,
      },
      {
        key: 'description',
        value:
          'A job that can be dispatched multiple times with different payloads and meta values',
      },
    ],
  },
  {
    id: 'nomad/job-templates/default/service-discovery',
    keyValues: [
      {
        key: 'template',
        value: serviceDiscovery,
      },
      {
        key: 'description',
        value:
          'Registers a service in one group, and discovers it in another. Provides a recurring check to ensure the service is healthy',
      },
    ],
  },
  {
    id: 'nomad/job-templates/default/variables',
    keyValues: [
      {
        key: 'template',
        value: variables,
      },
      {
        key: 'description',
        value:
          'Use Nomad Variables to configure the output of a simple HTML page',
      },
    ],
  },
];
