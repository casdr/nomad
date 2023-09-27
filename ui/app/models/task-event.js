/**
 * Copyright (c) HashiCorp, Inc.
 * SPDX-License-Identifier: BUSL-1.1
 */

import Fragment from 'ember-data-model-fragments/fragment';
import { attr } from '@ember-data/model';
import { fragmentOwner } from 'ember-data-model-fragments/attributes';

export default class TaskEvent extends Fragment {
  @fragmentOwner() state;

  @attr('string') type;
  @attr('number') signal;
  @attr('number') exitCode;

  @attr('date') time;
  @attr('number') timeNanos;
  @attr('string') displayMessage;

  get message() {
    let message = simplifyTimeMessage(this.displayMessage);
    return message;
  }
}

function simplifyTimeMessage(message) {
  return (
    message?.replace(/(\d+h)?(\d+m)?(\d+\.\d+)s/g, (_, h, m, s) => {
      h = h ? parseInt(h) : 0;
      m = m ? parseInt(m) : 0;
      s = Math.round(parseFloat(s));

      m += Math.floor(s / 60);
      s %= 60;
      h += Math.floor(m / 60);
      m %= 60;

      return `${h ? h + 'h' : ''}${h || m ? m + 'm' : ''}${s}s`;
    }) || message
  );
}
