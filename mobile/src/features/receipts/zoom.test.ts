import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  activeStep,
  applyStep,
  factorForZoom,
  hasUltraWideLens,
  pinchDistance,
  pinchZoom,
  ULTRA_WIDE,
  WIDE,
  zoomForFactor,
  zoomSteps,
} from './zoom.ts';

test('0,5× is offered only with an ultra wide lens', () => {
  assert.deepEqual(zoomSteps(false).map((s) => s.label), ['1×', '2×']);
  assert.deepEqual(zoomSteps(true).map((s) => s.label), ['0,5×', '1×', '2×']);
  assert.equal(hasUltraWideLens(undefined), false);
  assert.equal(hasUltraWideLens([{ deviceType: WIDE }]), false);
  assert.equal(hasUltraWideLens([{ deviceType: WIDE }, { deviceType: ULTRA_WIDE }]), true);
});

test('factors map to expo-camera zoom and back', () => {
  assert.equal(zoomForFactor(1), 0);
  assert.equal(zoomForFactor(0.5), 0);
  assert.equal(zoomForFactor(8), 1);
  assert.equal(zoomForFactor(20), 1);
  assert.ok(Math.abs(zoomForFactor(2) - 1 / 7) < 1e-9);
  assert.ok(Math.abs(factorForZoom(zoomForFactor(2)) - 2) < 1e-9);
  assert.equal(factorForZoom(-1), 1);
});

test('quick buttons pick the lens and zoom', () => {
  assert.deepEqual(applyStep(0.5, true), { lens: ULTRA_WIDE, zoom: 0 });
  // Without an ultra wide lens 0,5× falls back to 1×.
  assert.deepEqual(applyStep(0.5, false), { lens: undefined, zoom: 0 });
  assert.deepEqual(applyStep(1, true), { lens: undefined, zoom: 0 });
  assert.equal(applyStep(2, false).zoom, zoomForFactor(2));
});

test('the active quick button follows the camera', () => {
  assert.equal(activeStep(0, false), 1);
  assert.equal(activeStep(zoomForFactor(2), false), 2);
  assert.equal(activeStep(0.5, false), null);
  assert.equal(activeStep(0, true), 0.5);
  assert.equal(activeStep(0.3, true), null);
});

test('pinch spreads to zoom in and stays within 0–1', () => {
  assert.equal(pinchDistance([{ pageX: 0, pageY: 0 }]), null);
  assert.equal(pinchDistance([{ pageX: 0, pageY: 0 }, { pageX: 30, pageY: 40 }]), 50);
  assert.equal(pinchZoom(0, 100, 100), 0);
  assert.ok(pinchZoom(0, 100, 300) > 0);
  assert.ok(pinchZoom(0.5, 300, 100) < 0.5);
  assert.equal(pinchZoom(0.1, 300, 0), 0);
  assert.equal(pinchZoom(0.9, 0, 2000), 1);
});
