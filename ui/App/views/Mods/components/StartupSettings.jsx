import React, {useEffect, useState} from "react";
import client from "../../../../api/client";
import Button from "../../../components/Button";
import Panel from "../../../components/Panel";
import UploadMod from "./UploadMod";

const StartupSettings = ({running}) => {
    const [document, setDocument] = useState(null);
    const [changes, setChanges] = useState({});
    const [query, setQuery] = useState("");
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState("");
    const [message, setMessage] = useState("");
    const count = Object.keys(changes).length;
    const load = () => {
        setBusy(true); setError(""); setMessage("");
        client.get('/api/mods/startup-settings')
            .then(({data}) => {setDocument(data); setChanges({});})
            .catch(e => setError(typeof e.response?.data === 'string' ? e.response.data : 'Could not load startup settings.'))
            .finally(() => setBusy(false));
    };
    useEffect(load, []);
    useEffect(() => {
        if (!count) return;
        const warn = e => {e.preventDefault(); e.returnValue = '';};
        window.addEventListener('beforeunload', warn);
        return () => window.removeEventListener('beforeunload', warn);
    }, [count]);
    const change = (setting, value, component) => {
        setMessage("");
        setChanges(previous => {
            const next = {...previous};
            if (component) {
                const values = {...(next[setting.name]?.components || {}), [component.name]: value};
                if (value === component.value) delete values[component.name];
                if (Object.keys(values).length) next[setting.name] = {components: values};
                else delete next[setting.name];
            } else if (value === setting.value) delete next[setting.name];
            else next[setting.name] = {value};
            return next;
        });
    };
    const save = e => {
        e.preventDefault(); setBusy(true); setError(""); setMessage("");
        client.post('/api/mods/startup-settings', {revision: document.revision, changes})
            .then(({data}) => {setDocument(data); setChanges({}); setMessage('Startup settings saved. They take effect the next time you start the server.');})
            .catch(e => setError(typeof e.response?.data === 'string' ? e.response.data : 'Could not save settings. Your edits are still here.'))
            .finally(() => setBusy(false));
    };
    const input = (setting, component) => {
        const field = component || setting;
        const value = component ? changes[setting.name]?.components?.[field.name] ?? field.value : changes[setting.name]?.value ?? field.value;
        const label = component ? `${setting.name} ${field.name}` : setting.name;
        if (field.type === 'bool') return <label className="flex items-center gap-2">
            <input aria-label={label} type="checkbox" checked={value === 'true'} disabled={running || busy}
                   onChange={e => change(setting, String(e.target.checked), component)}/>
            <span>{value === 'true' ? 'Enabled' : 'Disabled'}</span>
        </label>;
        return <input aria-label={label} className="w-full text-black px-3 py-2 rounded-sm"
                      type={field.type === 'number' || component ? 'number' : 'text'}
                      step="any" min={component ? 0 : undefined} max={component ? 1 : undefined}
                      inputMode={['int', 'uint'].includes(field.type) ? 'numeric' : undefined}
                      pattern={field.type === 'int' ? '-?[0-9]+' : field.type === 'uint' ? '[0-9]+' : undefined}
                      required={field.type !== 'string'} value={value} disabled={running || busy}
                      onChange={e => change(setting, e.target.value, component)}/>;
    };
    return <Panel title="Startup settings" className="mb-6" content={<>
        <p className="mb-3">Edit the startup values stored on this server. Saving backs up the current file and leaves runtime settings unchanged. Mod-specific limits still apply when Factorio loads.</p>
        {running && <p className="text-orange mb-3" role="status">Stop the server before saving startup settings.</p>}
        {error && <p role="alert" className="text-red mb-3">{error}</p>}
        {message && <p role="status" className="text-green mb-3">{message}</p>}
        {!document && <UploadMod settingsOnly disabled={running || busy} refetchInstalledMods={load}/>}
        {!document && <Button onClick={load} isLoading={busy}>Reload settings</Button>}
        {document && <form onSubmit={save}>
            <div className="flex flex-wrap items-center gap-3 mb-4">
                <input aria-label="Search startup settings" placeholder="Search settings…" className="text-black px-3 py-2 rounded-sm flex-grow" value={query} onChange={e => setQuery(e.target.value)}/>
                <Button isSubmit type="success" isLoading={busy} isDisabled={running || !count}>Save {count ? `${count} change${count === 1 ? '' : 's'}` : 'changes'}</Button>
                <Button isDisabled={busy || !count} onClick={() => {setChanges({});setError("");setMessage("");}}>Discard edits</Button>
                <Button isDisabled={busy} onClick={() => {if (!count || window.confirm('Discard your unsaved edits and reload settings?')) load();}}>Reload</Button>
            </div>
            <p className="text-sm mb-3">{document.settings.length} saved startup settings · {count} changed</p>
            {!document.settings.length && <p>No startup settings are stored in this file.</p>}
            {document.settings.length > 0 && !document.settings.some(s => s.name.toLowerCase().includes(query.toLowerCase())) && <p>No settings match your search.</p>}
            {document.settings.filter(s => s.name.toLowerCase().includes(query.toLowerCase())).map(setting =>
                <div key={setting.name} className="border-b border-gray-light py-4 flex flex-wrap items-center gap-4">
                    <div className="flex-1 min-w-0"><div className="font-bold break-words">{setting.name}{changes[setting.name] ? ' *' : ''}</div><div className="text-sm">{setting.type}</div></div>
                    <div className="w-full sm:w-1/2">
                        {setting.type === 'unsupported' ? <span>This value type is read-only.</span> : setting.type === 'color' ? <div className="grid grid-cols-2 gap-2">{setting.components.map(c => <label key={c.name}>{c.name.toUpperCase()}{input(setting,c)}</label>)}</div> : input(setting)}
                    </div>
                </div>)}
        </form>}
    </>}/>;
};
export default StartupSettings;
