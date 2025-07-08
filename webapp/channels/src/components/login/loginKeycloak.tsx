import React from 'react';
import {useIntl} from 'react-intl';

export default function LoginKeycloakIcon(props: React.HTMLAttributes<HTMLSpanElement>) {
    const {formatMessage} = useIntl();

    return (
        <span {...props}>
            <svg
                width='16'
                height='16'
                viewBox='0 0 24 24'
                fill='currentColor'
                xmlns='http://www.w3.org/2000/svg'
                aria-label={formatMessage({id: 'generic_icons.login.keycloak', defaultMessage: 'Keycloak Icon'})}
            >
                <path d="..." /> {/* Можно вставить логотип из https://simpleicons.org/icons/keycloak.svg */}
            </svg>
        </span>
    );
}
