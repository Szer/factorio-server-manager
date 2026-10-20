import Panel from "../components/Panel";
import React, {useEffect, useState} from "react";
import settingsResource from "../../api/resources/settings";
import Input from "../components/Input";
import Label from "../components/Label";
import Checkbox from "../components/Checkbox";
import InputPassword from "../components/InputPassword";
import Button from "../components/Button";
import {useForm} from "react-hook-form";

const ServerSettings = () => {
    const [settings, setSettings] = useState();
    const {register, handleSubmit, reset, formState: {isSubmitting}} = useForm();

    const fetchSettings = async () => {
        const res = await settingsResource.server.list();
        const values = {...res};
        for (const [key, value] of Object.entries(res)) {
            if (Array.isArray(value) && value.every(item => typeof item === "string")) {
                values[key] = value.join(",");
            }
        }
        setSettings(res);
        reset(values);
    };

    const reportError = error => {
        // HTTP failures are displayed by the shared API client.
        if (!error.isAxiosError) window.flash("Could not load or save server settings.", "red");
    };

    const saveServerSettings = async data => {
        try {
            const updated = {...settings};
            for (const [key, value] of Object.entries(settings)) {
                if (key.startsWith("_comment") || data[key] === undefined) continue;
                if (Array.isArray(value) && value.every(item => typeof item === "string")) {
                    updated[key] = String(data[key] ?? "").split(",").map(item => item.trim()).filter(Boolean);
                } else if (key === "visibility" && value && typeof value === "object") {
                    updated[key] = {...value, ...data[key]};
                } else if (["string", "number", "boolean"].includes(typeof value)) {
                    updated[key] = data[key];
                }
            }
            await settingsResource.server.update(updated);
            await fetchSettings();
            window.flash("Settings saved.", "green");
        } catch (error) {
            reportError(error);
        }
    };

    useEffect(() => { fetchSettings().catch(reportError); }, []);

    const formTypeField = (name, value, label) => {
        switch (typeof value) {
            case "number":
                return <>
                    <Label htmlFor={name} text={label}/>
                    <Input type="number" step="any" register={register(name, {valueAsNumber: true, validate: Number.isFinite})}/>
                </>;
            case "string":
                return <>
                    <Label htmlFor={name} text={label}/>
                    {name.includes("password")
                        ? <InputPassword register={register(name)}/>
                        : <Input register={register(name)}/>}
                </>;
            case "boolean":
                return <Checkbox text={label} register={register(name)}/>;
            case "object":
                if (Array.isArray(value) && value.every(item => typeof item === "string")) {
                    return <>
                        <Label htmlFor={name} text={label}/>
                        <Input register={register(name)}/>
                    </>;
                }
                if (name === "visibility" && value) {
                    return <>
                        <Label text="Visibility"/>
                        <div className="flex">
                            {Object.keys(value).map(key => <div className="mr-4" key={key}>
                                <Checkbox register={register(`visibility.${key}`)} text={key}/>
                            </div>)}
                        </div>
                    </>;
                }
                return null;
            default:
                return null;
        }
    };

    return <form className="mb-4" onSubmit={handleSubmit(saveServerSettings,
        () => window.flash("Check server settings: numeric fields must contain valid numbers.", "red"))}>
        <Panel title="Server Settings" content={
            settings && Object.keys(settings).filter(key => !key.startsWith("_comment")).map(key =>
                <div className="mb-4" key={key}>
                    {formTypeField(key, settings[key], key.replaceAll("_", " "))}
                    <p className="text-sm italic">{settings["_comment_" + key]}</p>
                </div>)
        } actions={
            <Button isSubmit={true} type="success" isLoading={isSubmitting} isDisabled={!settings}>Save</Button>
        }/>
    </form>;
};

export default ServerSettings;
