import http from 'k6/http';
import { check, sleep } from 'k6';
import { pickBusiness } from '../lib/fixtures.js';
import { menuBrowseThresholds } from '../lib/thresholds.js';

const baseURL = __ENV.BASE_URL || 'http://localhost:8080';

export const options = {
  thresholds: menuBrowseThresholds,
  scenarios: {
    menu_browse: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS) || 10,
      duration: __ENV.DURATION || '30s',
    },
  },
};

export default function () {
  const slug = pickBusiness(__VU, __ITER);
  const res = http.get(`${baseURL}/api/v1/business/${slug}/menu`);
  check(res, { 'menu 200': r => r.status === 200 });
  sleep(Math.random() * 0.5);
}
