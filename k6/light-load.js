import http from 'k6/http';
import { check, sleep } from 'k6';

const baseUrl = __ENV.BASE_URL || 'http://localhost:8080';
const batchSize = Number(__ENV.BATCH_SIZE || 25);
const hotAccountCount = Number(__ENV.HOT_ACCOUNT_COUNT || 20);

export const options = {
  scenarios: {
    light_load: {
      executor: 'ramping-vus',
      startVUs: Number(__ENV.START_VUS || 1),
      stages: [
        { duration: __ENV.RAMP_UP || '30s', target: Number(__ENV.TARGET_VUS || 5) },
        { duration: __ENV.STEADY || '60s', target: Number(__ENV.TARGET_VUS || 5) },
        { duration: __ENV.RAMP_DOWN || '15s', target: 0 },
      ],
      gracefulRampDown: '5s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<750'],
  },
};

function makeBatch(size, hotAccounts) {
  const txs = [];

  for (let i = 0; i < size; i++) {
    txs.push({
      id: `${__VU}-${__ITER}-${i}`,
      account_id: `acct-${i % hotAccounts}`,
      amount_cents: (i % 5000) + 1,
      currency: 'USD',
      ts: new Date().toISOString(),
    });
  }

  return JSON.stringify(txs);
}

export default function () {
  const payload = makeBatch(batchSize, hotAccountCount);

  const res = http.post(`${baseUrl}/api/v1/transactions`, payload, {
    headers: { 'Content-Type': 'application/json' },
    timeout: '2s',
  });

  check(res, {
    accepted: (r) => r.status === 202,
  });

  sleep(Number(__ENV.SLEEP_SECONDS || 0.5));
}
