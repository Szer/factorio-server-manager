import React, {useState} from "react";
import Button from "../../../components/Button";
import Label from "../../../components/Label";
import modsResource from "../../../../api/resources/mods";

const UploadMod = ({refetchInstalledMods, settingsOnly = false, disabled = false}) => {
    const [file, setFile] = useState(null);
    const [isUploading, setIsUploading] = useState(false);
    const [error, setError] = useState("");
    const [message, setMessage] = useState("");
    const id = settingsOnly ? 'startup_settings_file' : 'mod_file';
    const onSubmit = async e => {
        e.preventDefault();
        if (!file || disabled || isUploading) return;
        const form = e.currentTarget;
        setError(""); setMessage("");
        if (settingsOnly ? file.name !== 'mod-settings.dat' :
            !(/\.zip$/i.test(file.name) || ['mod-settings.dat', 'mod-list.json'].includes(file.name))) {
            setError(settingsOnly ? 'Select a file named mod-settings.dat.' : 'Select a mod ZIP, mod-settings.dat, or mod-list.json.');
            return;
        }
        setIsUploading(true);
        try {
            await modsResource.upload(file);
            setMessage(`${file.name} uploaded successfully.`);
            form.reset(); setFile(null);
            if (refetchInstalledMods) await refetchInstalledMods();
        } catch (e) {
            setError(typeof e.response?.data === 'string' ? e.response.data : 'Upload failed. Check the server and try again.');
        } finally {
            setIsUploading(false);
        }
    };
    return <form onSubmit={onSubmit} className="mb-4">
        <p className="mb-3">{settingsOnly
            ? 'Import mod-settings.dat from your previous server or Factorio client mods folder. This replaces all stored mod settings, including runtime settings.'
            : 'Upload individual mod ZIPs, then mod-settings.dat and mod-list.json from your previous server or client. Upload mod-list.json last to preserve which mods are disabled. A save ZIP does not install its mods.'}</p>
        <Label text={settingsOnly ? 'mod-settings.dat' : 'Mod ZIP or settings file'} htmlFor={id}/>
        <div className="relative bg-white shadow text-black mb-4">
            <input id={id} type="file" disabled={disabled || isUploading}
                className="absolute left-0 top-0 opacity-0 cursor-pointer w-full h-full"
                accept={settingsOnly ? '.dat' : '.zip,.dat,.json'}
                onChange={e => {setFile(e.currentTarget.files?.[0] || null); setError(""); setMessage("");}}/>
            <div className="px-2 py-2">{file?.name || 'Select file…'}</div>
        </div>
        {error && <p role="alert" className="text-red mb-3">{error}</p>}
        {message && <p role="status" className="text-green mb-3">{message}</p>}
        <Button isLoading={isUploading} isDisabled={disabled || !file} isSubmit>Upload</Button>
    </form>;
};
export default UploadMod;
