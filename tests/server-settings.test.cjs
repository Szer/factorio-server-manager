const {test} = require('node:test');
const assert = require('node:assert/strict');
const {readFileSync} = require('node:fs');
const path = require('node:path');
const {JSDOM} = require('jsdom');
const {transformSync} = require('@babel/core');

const dom = new JSDOM('<!doctype html><html><body></body></html>', {url: 'http://localhost'});
for (const key of ['window', 'document', 'navigator', 'HTMLElement', 'Element', 'FileList']) {
    Object.defineProperty(global, key, {value: dom.window[key], configurable: true});
}
global.IS_REACT_ACT_ENVIRONMENT = true;
const uiRoot = path.resolve('ui') + path.sep;
const originalJS = require.extensions['.js'];
function compile(module, filename) {
    if (!filename.startsWith(uiRoot)) return originalJS(module, filename);
    const result = transformSync(readFileSync(filename, 'utf8'), {
        filename, babelrc: false, configFile: false,
        presets: ['@babel/preset-react'], plugins: ['@babel/plugin-transform-modules-commonjs'],
    });
    module._compile(result.code, filename);
}
require.extensions['.jsx'] = compile;
require.extensions['.js'] = compile;
const React = require('react');
const {createRoot} = require('react-dom/client');
const {act, Simulate} = require('react-dom/test-utils');
const {AxiosError} = require('axios');
const client = require('../ui/api/client').default;
const ServerSettings = require('../ui/App/views/ServerSettings.jsx').default;

async function harness(t) {
    let stored = {
        admins: ['old-admin'], tags: ['old-tag'], name: 'Old name', game_password: 'old-password',
        auto_pause: true, autosave_interval: 2.5, visibility: {public: true, lan: false},
        _comment_admins: 'Names of administrators', _comment_visibility: 'Server visibility',
        unknown: {keep: 'unchanged'}, nullable: null,
    };
    const posts = [], flashes = [];
    let fail = false, gets = 0;
    const previousAdapter = client.defaults.adapter;
    window.flash = (...args) => flashes.push(args);
    client.defaults.adapter = async config => {
        if (config.method === 'post') {
            posts.push(JSON.parse(config.data));
            if (fail) throw new AxiosError('Save failed', 'ERR_BAD_RESPONSE', config, null,
                {status: 500, data: 'Failed to save server settings: permission denied', config});
            stored = JSON.parse(config.data);
            return {status: 200, data: 'Settings successfully saved', config};
        }
        gets++;
        return {status: 200, data: structuredClone(stored), config};
    };
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root = createRoot(container);
    await act(async () => { root.render(React.createElement(ServerSettings)); });
    t.after(async () => {
        await act(async () => root.unmount());
        container.remove();
        client.defaults.adapter = previousAdapter;
    });
    const input = name => {
        const element = container.querySelector(`[name="${name}"]`);
        assert.ok(element, `registered input ${name}`);
        return element;
    };
    return {
        posts, flashes, input, get gets() {return gets;},
        fail: () => { fail = true; },
        change: async (name, value) => {
            const element = input(name);
            await act(async () => {
                if (element.type === 'checkbox') element.checked = value;
                else element.value = value;
                Simulate.change(element);
            });
        },
        save: async () => {
            await act(async () => { Simulate.submit(container.querySelector('form')); });
        },
        remount: async () => {
            await act(async () => root.unmount());
            root = createRoot(container);
            await act(async () => root.render(React.createElement(ServerSettings)));
        },
    };
}

test('settings edits submit registered fields and survive reload', async t => {
    const h = await harness(t);
    assert.equal(h.input('admins').value, 'old-admin');
    assert.equal(h.input('auto_pause').checked, true);
    await h.change('admins', ' Szer, paragith, , ');
    await h.change('tags', ' factory, space ');
    await h.change('name', 'New name');
    await h.change('game_password', 'new-password');
    await h.change('autosave_interval', '3.75');
    await h.change('auto_pause', false);
    await h.change('visibility.public', false);
    await h.change('visibility.lan', true);
    await h.save();
    assert.equal(h.posts.length, 1);
    assert.deepEqual(h.posts[0], {
        admins: ['Szer', 'paragith'], tags: ['factory', 'space'], name: 'New name', game_password: 'new-password',
        auto_pause: false, autosave_interval: 3.75, visibility: {public: false, lan: true},
        _comment_admins: 'Names of administrators', _comment_visibility: 'Server visibility',
        unknown: {keep: 'unchanged'}, nullable: null,
    });
    assert.deepEqual(h.flashes, [['Settings saved.', 'green']]);
    assert.equal(h.input('admins').value, 'Szer,paragith');
    await h.remount();
    assert.equal(h.input('admins').value, 'Szer,paragith');
    assert.equal(h.input('autosave_interval').value, '3.75');
    assert.equal(h.input('visibility.lan').checked, true);
});

test('empty lists serialize to empty arrays', async t => {
    const h = await harness(t);
    await h.change('admins', ' , ');
    await h.change('tags', '');
    await h.save();
    assert.deepEqual(h.posts[0].admins, []);
    assert.deepEqual(h.posts[0].tags, []);
});

test('failed saves show the API error and keep edits without reporting success', async t => {
    const h = await harness(t);
    h.fail();
    await h.change('admins', 'unsaved-admin');
    await h.save();
    assert.equal(h.posts.length, 1);
    assert.equal(h.gets, 1);
    assert.equal(h.input('admins').value, 'unsaved-admin');
    assert.deepEqual(h.flashes, [['Failed to save server settings: permission denied', 'red']]);
});

test('invalid numbers prevent posting and display validation feedback', async t => {
    const h = await harness(t);
    await h.change('autosave_interval', '');
    await h.save();
    assert.equal(h.posts.length, 0);
    assert.equal(h.flashes[0][1], 'red');
});
